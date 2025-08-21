// Copyright Jetstack Ltd. See LICENSE for details.
package passthrough

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	ginkgo "github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"

	authnv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"

	"github.com/jetstack/kube-oidc-proxy/test/e2e/framework"
)

var _ = framework.CasesDescribe("Audit", func() {
	f := framework.NewDefaultFramework("audit")

	ginkgo.It("should be able to write audit logs to file", func() {
		ginkgo.By("Creating policy file ConfigMap")
		cm, err := f.Helper().KubeClient.CoreV1().ConfigMaps(f.Namespace.Name).Create(context.TODO(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "kube-oidc-proxy-policy-",
			},
			Data: map[string]string{
				"audit.yaml": `apiVersion: audit.k8s.io/v1
kind: Policy
rules:
- level: RequestResponse`,
			},
		}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		vols := []corev1.Volume{
			corev1.Volume{
				Name: "audit",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cm.Name,
						},
					},
				},
			},
		}

		ginkgo.By("Deploying proxy with audit policy enabled")
		f.DeployProxyWith(vols, "--audit-log-path=/audit-log", "--audit-policy-file=/audit/audit.yaml")

		testAuditLogs(f, "app=kube-oidc-proxy-e2e")
	})

	ginkgo.It("should be able to write audit logs to webhook", func() {
		ginkgo.	By("Creating policy file ConfigMap")
		cmPolicy, err := f.Helper().KubeClient.CoreV1().ConfigMaps(f.Namespace.Name).Create(context.TODO(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "kube-oidc-proxy-policy-",
			},
			Data: map[string]string{
				"audit.yaml": `apiVersion: audit.k8s.io/v1
kind: Policy
rules:
- level: RequestResponse`,
			},
		}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		extraWebhookVol, webhookURL, err := f.Helper().DeployAuditWebhook(f.Namespace.Name, "/audit-log")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		cmWebhook, err := f.Helper().KubeClient.CoreV1().ConfigMaps(f.Namespace.Name).Create(context.TODO(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "kube-oidc-proxy-webhook-config-",
			},
			Data: map[string]string{
				"kubeconfig.yaml": `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: ` + webhookURL.String() + `
    certificate-authority: /audit-webhook-ca/ca.pem
  name: logstash
contexts:
- context:
    cluster: logstash
    user: ""
  name: default-context
current-context: default-context
preferences: {}
users: []`,
			},
		}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		vols := []corev1.Volume{
			corev1.Volume{
				Name: "audit",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cmPolicy.Name,
						},
					},
				},
			},
			corev1.Volume{
				Name: "audit-webhook",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cmWebhook.Name,
						},
					},
				},
			},
			extraWebhookVol,
		}

		ginkgo.By("Deploying proxy with audit policy enabled")
		f.DeployProxyWith(vols, "--audit-webhook-config-file=/audit-webhook/kubeconfig.yaml",
			"--audit-policy-file=/audit/audit.yaml", "--audit-webhook-initial-backoff=1s", "--audit-webhook-batch-max-wait=1s")

		testAuditLogs(f, "app=audit-webhook-e2e")
	})
})

func testAuditLogs(f *framework.Framework, podLabelSelector string) {
	ginkgo.By("Making calls to proxy to ensure audit get created")
	token := f.Helper().NewTokenPayload(f.IssuerURL(), f.ClientID(), time.Now().Add(time.Second*5))
	signedToken, err := f.Helper().SignToken(f.IssuerKeyBundle(), token)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	proxyConfig := f.NewProxyRestConfig()
	requester := f.Helper().NewRequester(proxyConfig.Transport, signedToken)

	target := fmt.Sprintf("%s/api/v1/namespaces/kube-system/pods", proxyConfig.Host)

	// Make request that should succeed
	_, _, err = requester.Get(target)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Make request that should be unauthenticated
	requester = f.Helper().NewRequester(proxyConfig.Transport, "foo")
	_, resp, err := requester.Get(target)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	if resp.StatusCode != http.StatusUnauthorized {
		gomega.Expect(fmt.Errorf("expected to get unauthorized, got=%d", resp.StatusCode)).NotTo(gomega.HaveOccurred())
	}

	ginkgo.By("Waiting for audit logs to be written")
	// 5 seconds here is longer than the proxy flush interval.
	time.Sleep(time.Second * 5)

	ginkgo.By("Copying audit log from proxy locally")
	// Get pod
	pods, err := f.Helper().KubeClient.CoreV1().Pods(f.Namespace.Name).List(context.TODO(), metav1.ListOptions{
		LabelSelector: podLabelSelector,
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	if len(pods.Items) != 1 {
		gomega.Expect(fmt.Errorf("expected single kube-oidc-proxy pod running, got=%d", len(pods.Items))).NotTo(gomega.HaveOccurred())
	}

	var auditLogsBuffer bytes.Buffer
	err = f.Helper().Kubectl(f.Namespace.Name).RunWithStdout(&auditLogsBuffer,
		"exec", pods.Items[0].Name, "cat", "/audit-log")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	logs := auditLogsBuffer.Bytes()
	scanner := bufio.NewScanner(bytes.NewReader(logs))

	expAuditEvents := []auditv1.Event{
		auditv1.Event{
			Level:      auditv1.LevelRequestResponse,
			Stage:      auditv1.StageRequestReceived,
			RequestURI: "/api/v1/namespaces/kube-system/pods",
			Verb:       "get",
			User: authnv1.UserInfo{
				Username: "user@example.com",
				Groups:   []string{"group-1", "group-2"},
			},
		},
		auditv1.Event{
			Level:      auditv1.LevelRequestResponse,
			Stage:      auditv1.StageResponseComplete,
			RequestURI: "/api/v1/namespaces/kube-system/pods",
			Verb:       "get",
			User: authnv1.UserInfo{
				Username: "user@example.com",
				Groups:   []string{"group-1", "group-2"},
			},
			ResponseStatus: &metav1.Status{
				Code: 403,
			},
		},

		// From what I could tell, this could never had succeeded - even pre-fork
		// auditv1.Event{
		// 	Level:      auditv1.LevelRequestResponse,
		// 	Stage:      auditv1.StageResponseStarted,
		// 	RequestURI: "/api/v1/namespaces/kube-system/pods",
		// 	Verb:       "get",
		// 	ResponseStatus: &metav1.Status{
		// 		Code:    401,
		// 		Message: "Authentication failed, attempted: bearer",
		// 	},
		// },
	}

	ginkgo.By("Testing for expected audit logs")
	var i int
	for scanner.Scan() {
		if i > len(expAuditEvents) {
			gomega.Expect(fmt.Errorf("more proxy audit logs than expected, exp=%d got=%s", len(expAuditEvents), logs)).NotTo(gomega.HaveOccurred())
		}

		var auditEvent auditv1.Event
		err = json.Unmarshal(scanner.Bytes(), &auditEvent)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		gotAuditEvent := auditv1.Event{
			Level:      auditEvent.Level,
			Stage:      auditEvent.Stage,
			RequestURI: auditEvent.RequestURI,
			Verb:       auditEvent.Verb,
			User: authnv1.UserInfo{
				Username: auditEvent.User.Username,
				Groups:   auditEvent.User.Groups,
			},
		}

		if auditEvent.ResponseStatus != nil {
			gotAuditEvent.ResponseStatus = &metav1.Status{
				Code:    auditEvent.ResponseStatus.Code,
				Message: auditEvent.ResponseStatus.Message,
			}
		}

		if !reflect.DeepEqual(expAuditEvents[i], gotAuditEvent) {
			gomega.Expect(fmt.Errorf("unexpected audit event\nexp=%v\ngot=%v", expAuditEvents[i], gotAuditEvent)).NotTo(gomega.HaveOccurred())
		}

		i++
	}

	if i != len(expAuditEvents) {
		gomega.Expect(fmt.Errorf("less proxy audit logs then expected, exp=%d, got=%s", len(expAuditEvents), logs)).NotTo(gomega.HaveOccurred())
	}
}

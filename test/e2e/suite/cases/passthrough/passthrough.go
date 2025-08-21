// Copyright Jetstack Ltd. See LICENSE for details.
package passthrough

import (
	"context"
	"fmt"
	"net/http"

	ginkgo "github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/jetstack/kube-oidc-proxy/test/e2e/framework"
)

var _ = framework.CasesDescribe("Passthrough", func() {
	f := framework.NewDefaultFramework("passthrough")

	var saToken string

	ginkgo.JustBeforeEach(func() {
		ginkgo.By("Creating List Pods Role")
		_, err := f.Helper().KubeClient.RbacV1().Roles(f.Namespace.Name).Create(context.TODO(),
			&rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name: "e2e-impersonation-pods-list",
				},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"pods"},
						Verbs:     []string{"get", "list"},
					},
				},
			}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Create bindings for both the OIDC user and default ServiceAccount
		ginkgo.By("Creating List Pods RoleBinding")
		_, err = f.Helper().KubeClient.RbacV1().RoleBindings(f.Namespace.Name).Create(context.TODO(),
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: "e2e-impersonation-pods-list",
				},
				Subjects: []rbacv1.Subject{
					{Name: "user@example.com", Kind: "User"},
					{Name: "default", Kind: "ServiceAccount"},
				},
				RoleRef: rbacv1.RoleRef{
					Name: "e2e-impersonation-pods-list", Kind: "Role"},
			}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Geting the token for the default ServiceAccount")
		sec, err := f.Helper().GetServiceAccountSecret(f.Namespace.Name, "default")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		saTokenBytes, ok := sec.Data[corev1.ServiceAccountTokenKey]
		if !ok {
			err = fmt.Errorf("expected token to be present in secret %s/%s (%s): %+v",
				sec.Name, sec.Namespace, corev1.ServiceAccountTokenKey, sec.Data)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}

		saToken = string(saTokenBytes)
	})

	ginkgo.JustAfterEach(func() {
		ginkgo.By("Deleting List Pods Role")
		err := f.Helper().KubeClient.RbacV1().Roles(f.Namespace.Name).Delete(context.TODO(),
			"e2e-impersonation-pods-list", metav1.DeleteOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Creating List Pods RoleBinding")
		err = f.Helper().KubeClient.RbacV1().RoleBindings(f.Namespace.Name).Delete(context.TODO(),
			"e2e-impersonation-pods-list", metav1.DeleteOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.It("error when a valid OIDC token is used but return correct when passthrough is disabled", func() {
		ginkgo.By("A valid OIDC token should respond without error")
		proxyClient := f.NewProxyClient()
		_, err := proxyClient.CoreV1().Pods(f.Namespace.Name).List(context.TODO(), metav1.ListOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Using a ServiceAccount token should error by the proxy")

		// Create requester using the ServiceAccount token
		config := f.NewProxyRestConfig()
		config.BearerToken = saToken

		client, err := kubernetes.NewForConfig(config)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		_, err = client.CoreV1().Pods(f.Namespace.Name).List(context.TODO(), metav1.ListOptions{})
		kErr, ok := err.(*k8sErrors.StatusError)
		if !ok {
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}

		expRespBody := "Unauthorized"
		resp := kErr.Status().Details.Causes[0].Message

		// Check body and status code the token was rejected
		if int(kErr.Status().Code) != http.StatusUnauthorized ||
			resp != expRespBody {
			gomega.Expect(fmt.Errorf("expected status code %d with body %q, got= %d %q",
				http.StatusUnauthorized, expRespBody, int(kErr.Status().Code), resp)).NotTo(gomega.HaveOccurred())
		}
	})

	ginkgo.It("should not error on a valid OIDC token nor a valid ServiceAccount token with passthrough enabled", func() {
		ginkgo.By("Enabling passthrough with Audience of the API Server")
		f.DeployProxyWith(nil, "--token-passthrough")

		ginkgo.By("A valid OIDC token should respond without error")
		proxyClient := f.NewProxyClient()
		_, err := proxyClient.CoreV1().Pods(f.Namespace.Name).List(context.TODO(), metav1.ListOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Using a ServiceAccount token should not error")

		// Create kube client using ServiceAccount token
		proxyConfig := f.NewProxyRestConfig()
		proxyConfig.BearerToken = saToken
		kubeProxyClient, err := kubernetes.NewForConfig(proxyConfig)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		_, err = kubeProxyClient.CoreV1().Pods(f.Namespace.Name).List(context.TODO(), metav1.ListOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})
})

// Copyright Jetstack Ltd. See LICENSE for details.
package headers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	ginkgo "github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"

	"github.com/jetstack/kube-oidc-proxy/test/e2e/framework"
	testutil "github.com/jetstack/kube-oidc-proxy/test/util"
)

var _ = framework.CasesDescribe("Headers", func() {
	f := framework.NewDefaultFramework("headers")

	ginkgo.JustAfterEach(func() {
		ginkgo.By("Deleting fake API Server")
		err := f.Helper().DeleteFakeAPIServer(f.Namespace.Name)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.It("should not respond with any extra headers if none are set on the proxy", func() {
		extraOIDCVolumes, fakeAPIServerURL, err := f.Helper().DeployFakeAPIServer(f.Namespace.Name)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Redeploying proxy to send traffic to fake API server")
		f.DeployProxyWith(extraOIDCVolumes, fmt.Sprintf("--server=%s", fakeAPIServerURL), "--certificate-authority=/fake-apiserver/ca.pem")

		resp := sendRequestToProxy(f)

		ginkgo.By("Ensuring no extra headers sent by proxy")
		for k := range resp.Header {
			if strings.HasPrefix(strings.ToLower(k), "impersonate-extra-") {
				gomega.Expect(fmt.Errorf("expected no extra user headers, got=%+v", resp.Header)).NotTo(gomega.HaveOccurred())
			}
		}
	})

	ginkgo.It("should respond with remote address and custom extra headers when they are set", func() {
		ginkgo.By("Deploying fake API Server")
		extraOIDCVolumes, fakeAPIServerURL, err := f.Helper().DeployFakeAPIServer(f.Namespace.Name)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Redeploying proxy to send traffic to fake API server with extra headers set")
		f.DeployProxyWith(extraOIDCVolumes, fmt.Sprintf("--server=%s", fakeAPIServerURL), "--certificate-authority=/fake-apiserver/ca.pem",
			"--extra-user-header-client-ip", "--extra-user-headers=key1=foo,key2=foo,key1=bar")

		resp := sendRequestToProxy(f)

		ginkgo.By("Ensuring expected headers are present")
		cpyHeader := resp.Header.Clone()

		// Check expected headers
		for k, v := range map[string][]string{
			"Impersonate-Extra-Key1": []string{"foo", "bar"},
			"Impersonate-Extra-Key2": []string{"foo"},
		} {
			if !testutil.StringSlicesEqual(v, cpyHeader[k]) {
				gomega.Expect(fmt.Errorf("expected key %q to have value %q, but got headers: %v",
					k, v, cpyHeader)).NotTo(gomega.HaveOccurred())
			}

			cpyHeader.Del(k)
		}

		// Check expected client IP header
		// TODO: determine a reliable way to get ip to match
		headerIP, ok := cpyHeader["Impersonate-Extra-Remote-Client-Ip"]
		if !ok || len(headerIP) != 1 {
			gomega.Expect(fmt.Errorf("expected impersonate extra remote client ip user header, got=%v", resp.Header)).NotTo(gomega.HaveOccurred())
		}

		cpyHeader.Del("Impersonate-Extra-Remote-Client-Ip")

		ginkgo.By("Ensuring no extra user headers where added")
		for k := range cpyHeader {
			if strings.HasPrefix(strings.ToLower(k), "impersonate-extra-") {
				gomega.Expect(fmt.Errorf("expected no impersonate extra user headers, got=%+v", resp.Header)).NotTo(gomega.HaveOccurred())
			}
		}
	})
})

func sendRequestToProxy(f *framework.Framework) *http.Response {
	ginkgo.By("Building request to proxy")
	tokenPayload := f.Helper().NewTokenPayload(
		f.IssuerURL(), f.ClientID(), time.Now().Add(time.Minute))

	signedToken, err := f.Helper().SignToken(f.IssuerKeyBundle(), tokenPayload)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	proxyConfig := f.NewProxyRestConfig()
	requester := f.Helper().NewRequester(proxyConfig.Transport, signedToken)

	ginkgo.By("Sending request to proxy")
	reqURL := fmt.Sprintf("%s/foo/bar", proxyConfig.Host)
	_, resp, err := requester.Get(reqURL)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return resp
}

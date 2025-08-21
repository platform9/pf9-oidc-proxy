// Copyright Jetstack Ltd. See LICENSE for details.
package token

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	ginkgo "github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"

	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jetstack/kube-oidc-proxy/test/e2e/framework"
)

var _ = framework.CasesDescribe("Token", func() {
	f := framework.NewDefaultFramework("token")

	ginkgo.It("should error when tokens are wrong for the issuer", func() {
		ginkgo.By("No token should error")
		expectProxyUnauthorized(f, nil)

		ginkgo.By("Bad token should error")
		expectProxyUnauthorized(f, []byte("bad token"))

		ginkgo.By("Wrong issuer should error")
		badURL, err := url.Parse("incorrect-issuer.io")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		expectProxyUnauthorized(f, f.Helper().NewTokenPayload(
			badURL, f.ClientID(), time.Now().Add(time.Minute)))

		ginkgo.By("Wrong audience should error")
		expectProxyUnauthorized(f, f.Helper().NewTokenPayload(
			f.IssuerURL(), "wrong-aud", time.Now().Add(time.Minute)))

		ginkgo.By("Token expires now")
		expectProxyUnauthorized(f, f.Helper().NewTokenPayload(
			f.IssuerURL(), f.ClientID(), time.Now()))

		ginkgo.By("Valid token should return Kubernetes forbidden")
		client := f.NewProxyClient()

		// If does not return with Kubernetes forbidden error then error
		_, err = client.CoreV1().Pods(f.Namespace.Name).List(context.TODO(), metav1.ListOptions{})
		if !k8sErrors.IsForbidden(err) {
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}
	})
})

func expectProxyUnauthorized(f *framework.Framework, tokenPayload []byte) {
	// Build client using given token payload
	signedToken, err := f.Helper().SignToken(f.IssuerKeyBundle(), tokenPayload)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	proxyConfig := f.NewProxyRestConfig()
	requester := f.Helper().NewRequester(proxyConfig.Transport, signedToken)

	// Send request with signed token to proxy
	target := fmt.Sprintf("%s/api/v1/namespaces/%s/pods",
		proxyConfig.Host, f.Namespace.Name)

	body, resp, err := requester.Get(target)
	body = bytes.TrimSpace(body)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Check body and status code the token was rejected
	if resp.StatusCode != http.StatusUnauthorized ||
		!bytes.Equal(body, []byte("Unauthorized")) {
		gomega.Expect(fmt.Errorf("expected status code %d with body Unauthorized, got= %d %q",
			http.StatusUnauthorized, resp.StatusCode, body)).NotTo(gomega.HaveOccurred())
	}
}

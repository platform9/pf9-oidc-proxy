// Copyright Jetstack Ltd. See LICENSE for details.
package probe

import (
	"time"

	ginkgo "github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"

	"github.com/jetstack/kube-oidc-proxy/test/e2e/framework"
	"github.com/jetstack/kube-oidc-proxy/test/kind"
)

var _ = framework.CasesDescribe("Readiness Probe", func() {
	f := framework.NewDefaultFramework("readiness-probe")

	ginkgo.It("Should not become ready if the issuer is unavailable", func() {
		ginkgo.By("Deleting the Issuer so no longer becomes reachable")
		gomega.Expect(f.Helper().DeleteIssuer(f.Namespace.Name)).NotTo(gomega.HaveOccurred())

		ginkgo.By("Deleting the current proxy that is ready")
		gomega.Expect(f.Helper().DeleteProxy(f.Namespace.Name)).NotTo(gomega.HaveOccurred())

		ginkgo.By("Deploying the Proxy should never become ready as the issuer is unavailable")
		_, _, err := f.Helper().DeployProxy(f.Namespace, f.IssuerURL(),
			f.ClientID(), f.IssuerKeyBundle(), nil)
		// Error should occur (not ready)
		gomega.Expect(err).To(gomega.HaveOccurred())
	})

	ginkgo.It("Should continue to be ready even if the issuer becomes unavailable", func() {
		ginkgo.By("Deleting the Issuer so no longer becomes reachable")
		gomega.Expect(f.Helper().DeleteIssuer(f.Namespace.Name)).NotTo(gomega.HaveOccurred())

		time.Sleep(time.Second * 10)

		err := f.Helper().WaitForDeploymentReady(f.Namespace.Name, kind.ProxyImageName, time.Second*5)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})
})

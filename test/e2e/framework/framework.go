// Copyright Jetstack Ltd. See LICENSE for details.
package framework

import (
	"fmt"
	"net/url"
	"time"

	ginkgo "github.com/onsi/ginkgo"
	gomega "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/jetstack/kube-oidc-proxy/test/e2e/framework/config"
	"github.com/jetstack/kube-oidc-proxy/test/e2e/framework/helper"
	"github.com/jetstack/kube-oidc-proxy/test/kind"
	"github.com/jetstack/kube-oidc-proxy/test/util"
)

var DefaultConfig = &config.Config{}

type Framework struct {
	BaseName string

	KubeClientSet kubernetes.Interface
	ProxyClient   kubernetes.Interface

	Namespace *corev1.Namespace

	config *config.Config
	helper *helper.Helper

	issuerKeyBundle, proxyKeyBundle *util.KeyBundle
	issuerURL, proxyURL             *url.URL
}

func NewDefaultFramework(baseName string) *Framework {
	return NewFramework(baseName, DefaultConfig)
}

func NewFramework(baseName string, config *config.Config) *Framework {
	f := &Framework{
		BaseName: baseName,
		config:   config,
	}

	ginkgo.JustBeforeEach(f.BeforeEach)
	ginkgo.AfterEach(f.AfterEach)

	return f
}

func (f *Framework) BeforeEach() {
	f.helper = helper.NewHelper(f.config)

	ginkgo.By("Creating a kubernetes client")

	clientConfigFlags := genericclioptions.NewConfigFlags(true)
	clientConfigFlags.KubeConfig = &f.config.KubeConfigPath
	config, err := clientConfigFlags.ToRESTConfig()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	f.KubeClientSet, err = kubernetes.NewForConfig(config)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("Building a namespace api object")
	f.Namespace, err = f.CreateKubeNamespace(f.BaseName)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("Using the namespace " + f.Namespace.Name)

	f.helper.KubeClient = f.KubeClientSet

	ginkgo.By("Deploying mock OIDC Issuer")
	issuerKeyBundle, issuerURL, err := f.helper.DeployIssuer(f.Namespace.Name)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("Deploying kube-oidc-proxy")
	proxyKeyBundle, proxyURL, err := f.helper.DeployProxy(f.Namespace,
		issuerURL, clientID, issuerKeyBundle, nil)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	f.issuerURL, f.proxyURL = issuerURL, proxyURL
	f.issuerKeyBundle, f.proxyKeyBundle = issuerKeyBundle, proxyKeyBundle

	ginkgo.By("Creating Proxy Client")
	f.ProxyClient = f.NewProxyClient()
}

// AfterEach deletes the namespace, after reading its events.
func (f *Framework) AfterEach() {
	// Output logs from proxy of test case.
	err := f.Helper().Kubectl(f.Namespace.Name).Run("logs", "-lapp=kube-oidc-proxy-e2e")
	if err != nil {
		ginkgo.By("Failed to gather logs from kube-oidc-proxy: " + err.Error())
	}

	ginkgo.By("Deleting kube-oidc-proxy deployment")
	err = f.Helper().DeleteProxy(f.Namespace.Name)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("Deleting mock OIDC issuer")
	err = f.Helper().DeleteIssuer(f.Namespace.Name)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("Deleting test namespace")
	err = f.DeleteKubeNamespace(f.Namespace.Name)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (f *Framework) DeployProxyWith(extraVolumes []corev1.Volume, extraArgs ...string) {
	ginkgo.By("Deleting kube-oidc-proxy deployment")
	err := f.Helper().DeleteProxy(f.Namespace.Name)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = f.Helper().WaitForDeploymentToDelete(f.Namespace.Name, kind.ProxyImageName, time.Second*30)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By(fmt.Sprintf("Deploying kube-oidc-proxy with extra args %s", extraArgs))
	f.proxyKeyBundle, f.proxyURL, err = f.helper.DeployProxy(f.Namespace, f.issuerURL,
		clientID, f.issuerKeyBundle, extraVolumes, extraArgs...)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (f *Framework) Helper() *helper.Helper {
	return f.helper
}

func (f *Framework) IssuerKeyBundle() *util.KeyBundle {
	return f.issuerKeyBundle
}

func (f *Framework) ProxyKeyBundle() *util.KeyBundle {
	return f.proxyKeyBundle
}

func (f *Framework) IssuerURL() *url.URL {
	return f.issuerURL
}

func (f *Framework) ProxyURL() *url.URL {
	return f.proxyURL
}

func (f *Framework) ClientID() string {
	return clientID
}

func (f *Framework) NewProxyRestConfig() *rest.Config {
	config, err := f.Helper().NewValidRestConfig(f.issuerKeyBundle, f.proxyKeyBundle,
		f.issuerURL, f.proxyURL, clientID)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return config
}

func (f *Framework) NewProxyClient() kubernetes.Interface {
	proxyConfig := f.NewProxyRestConfig()

	proxyClient, err := kubernetes.NewForConfig(proxyConfig)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return proxyClient
}

func CasesDescribe(text string, body func()) bool {
	return ginkgo.Describe("[TEST] "+text, body)
}

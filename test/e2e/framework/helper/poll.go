// Copyright Jetstack Ltd. See LICENSE for details.
package helper

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func (h *Helper) WaitForDeploymentReady(namespace, name string, timeout time.Duration) error {
	log.Infof("Waiting for Deployment to become ready %s/%s", namespace, name)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		deploy, err := h.KubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if deploy.Spec.Replicas == nil {
			return false, nil
		}

		return *deploy.Spec.Replicas == deploy.Status.ReadyReplicas, nil
	})

	if err != nil {
		if kErr := h.Kubectl(namespace).DescribeResource("deployment", name); kErr != nil {
			err = fmt.Errorf("%s\n%s", err, kErr)
		}
	}
	return err
}

func (h *Helper) WaitForPodReady(namespace, name string, timeout time.Duration) error {
	log.Infof("Waiting for Pod to become ready %s/%s", namespace, name)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := h.KubeClient.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if len(pod.Status.Conditions) == 0 {
			return false, nil
		}

		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				return true, nil
			}
		}

		log.Infof("helper: pod not ready %s/%s: %v", pod.Namespace, pod.Name, pod.Status.Conditions)
		return false, nil
	})
	if err != nil {
		if kErr := h.Kubectl(namespace).DescribeResource("pod", name); kErr != nil {
			err = fmt.Errorf("%s\n%s", err, kErr)
		}
	}
	return err
}

func (h *Helper) WaitForDeploymentToDelete(namespace, name string, timeout time.Duration) error {
	log.Infof("Waiting for Deployment to be deleted: %s/%s", namespace, name)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		_, err := h.KubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if k8sErrors.IsNotFound(err) {
			log.Infof("Deployment %s/%s deleted, waiting for pods", namespace, name)
			pods, err := h.KubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})

			if err != nil {
				return false, nil
			}

			for _, pod := range pods.Items {
				if strings.HasPrefix(pod.Name, name+"-") {
					log.Infof("Pod %s/%s still not terminated", namespace, pod.Name)
					return false, nil
				}
			}

			log.Infof("All pods for %s/%s terminated", namespace, name)
			return true, nil
		}

		if err != nil {
			return false, nil
		}

		return false, nil
	})

	if err != nil {
		if kErr := h.Kubectl(namespace).DescribeResource("deployment", name); kErr != nil {
			err = fmt.Errorf("%s\n%s", err, kErr)
		}
	}
	return err

}

func (h *Helper) WaitForUrlToBeReady(targetURL *url.URL, timeout time.Duration) error {
	log.Infof("Waiting for URL %s to be ready", targetURL)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		host := targetURL.Host
		if port := targetURL.Port(); port != "" {
			host = host + ":" + port
		}

		con, err := net.DialTimeout("tcp", host, timeout)
		if err != nil {
			return false, nil
		} else {
			defer func() {
					_ = con.Close()
			}()
			return true, nil
		}

	})

	return err
}

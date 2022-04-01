package framework

import (
	"context"
	"os"
	"time"

	helm "github.com/k3s-io/helm-controller/pkg/controllers/chart"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *Framework) setupController(ctx context.Context) error {
	_, err := f.ClientSet.CoreV1().Namespaces().Create(ctx, f.getNS(), metav1.CreateOptions{})
	if err != nil {
		return err
	}

	_, err = f.ClientSet.RbacV1().ClusterRoleBindings().Create(ctx, f.getCrb(), metav1.CreateOptions{})
	if err != nil {
		return err
	}

	if err := f.crdFactory.BatchCreateCRDs(ctx, f.crds...).BatchWait(); err != nil {
		return err
	}

	_, err = f.ClientSet.CoreV1().ServiceAccounts(f.Namespace).Create(ctx, f.getSa(), metav1.CreateOptions{})
	if err != nil {
		return err
	}

	err = wait.Poll(time.Second, 15*time.Second, func() (bool, error) {
		_, err := f.ClientSet.CoreV1().ServiceAccounts(f.Namespace).Get(ctx, f.Name, metav1.GetOptions{})
		if err == nil {
			return true, nil
		}

		if errors.IsNotFound(err) {
			return false, nil
		}

		logrus.Printf("Waiting for SA to be ready: %+v\n", err)
		return false, err
	})
	if err != nil {
		return err
	}

	_, err = f.ClientSet.AppsV1().Deployments(f.Namespace).Create(context.TODO(), f.getDeployment(), metav1.CreateOptions{})
	return err
}

func (f *Framework) getNS() *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.Name,
			Namespace: f.Namespace,
		},
	}
}

func (f *Framework) getDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.Name,
			Namespace: f.Namespace,
			Labels: map[string]string{
				"app": f.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": f.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": f.Name,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: f.Name,
					Containers: []corev1.Container{
						{
							Name:    f.Name,
							Image:   getImage(),
							Command: []string{"helm-controller"},
							Args:    []string{"--namespace", "helm-controller"},
						},
					},
				},
			},
		},
	}
}

func getImage() string {
	if img, ok := os.LookupEnv("HELM_CONTROLLER_IMAGE"); ok {
		return img
	}
	return "rancher/helm-controller:latest"
}

func (f *Framework) getCrb() *v1.ClusterRoleBinding {
	return &v1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: f.Name,
		},
		RoleRef: v1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []v1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      f.Name,
				Namespace: f.Namespace,
			},
		},
	}
}

func (f *Framework) getSa() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.Name,
			Namespace: f.Namespace,
		},
	}
}

func (f *Framework) teardownController(ctx context.Context) error {
	charts, err := f.ListHelmCharts("helm-test=true", f.Namespace)
	if err != nil {
		return err
	}
	for _, item := range charts.Items {
		// refresh object before updating; it may have changed since listing
		rItem, err := f.GetHelmChart(item.Name, item.Namespace)
		if err != nil {
			return err
		}

		rItem.Finalizers = []string{}
		_, err = f.UpdateHelmChart(rItem, f.Namespace)
		if err != nil {
			return err
		}

		err = f.DeleteHelmChart(item.Name, item.Namespace)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	err = f.ClientSet.RbacV1().ClusterRoleBindings().Delete(ctx, f.Name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	err = f.crdFactory.CRDClient.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, helm.CRDName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	err = f.ClientSet.CoreV1().Namespaces().Delete(ctx, f.Namespace, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	return nil
}

// Copyright 2019-2025 The Liqo Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package version

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;create;update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;create;update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;create;update

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/liqotech/liqo/pkg/utils/resource"
)

const (
	// LiqoVersionConfigMapName is the name of the ConfigMap containing the Liqo version.
	LiqoVersionConfigMapName = "liqo-version"
	// LiqoVersionReaderRoleName is the name of the Role that allows reading the liqo-version ConfigMap.
	LiqoVersionReaderRoleName = "liqo-version-reader"
	// LiqoVersionReaderRoleBindingName is the name of the RoleBinding for the liqo-version-reader Role.
	LiqoVersionReaderRoleBindingName = "liqo-version-reader-binding"
	// LiqoVersionKey is the key in the ConfigMap data where the version is stored.
	LiqoVersionKey = "version"
	// LiqoGroupName is the RBAC group name for Liqo identities.
	LiqoGroupName = "liqo.io"
)

// GetVersionFromDeployment reads the liqo-controller-manager deployment and extracts
// the version from its container image tag.
func GetVersionFromDeployment(ctx context.Context, clientset kubernetes.Interface, namespace, deploymentName string) string {
	deployment, err := clientset.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		klog.Warningf("Failed to get deployment %s/%s: %v", namespace, deploymentName, err)
		return ""
	}

	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		klog.Warning("No containers found in deployment")
		return ""
	}

	image := deployment.Spec.Template.Spec.Containers[0].Image

	// Extract the tag from the image (format: registry/org/image:tag)
	parts := strings.Split(image, ":")
	if len(parts) < 2 {
		klog.Warningf("Image %q does not contain a tag, cannot determine Liqo version", image)
		return ""
	}

	tag := parts[len(parts)-1]
	klog.Infof("Detected Liqo version from deployment: %s", tag)
	return tag
}

// SetupVersionResources creates or updates the ConfigMap, Role, and RoleBinding
// for exposing the Liqo version to remote clusters.
func SetupVersionResources(ctx context.Context, clientset kubernetes.Interface, liqoNamespace, version string) error {
	if version == "" {
		klog.Warning("Liqo version is empty, skipping version resources setup")
		return nil
	}

	// Create or update the ConfigMap
	if err := createOrUpdateVersionConfigMap(ctx, clientset, liqoNamespace, version); err != nil {
		return fmt.Errorf("failed to create/update version ConfigMap: %w", err)
	}

	// Create or update the Role
	if err := createOrUpdateVersionReaderRole(ctx, clientset, liqoNamespace); err != nil {
		return fmt.Errorf("failed to create/update version reader Role: %w", err)
	}

	// Create or update the RoleBinding
	if err := createOrUpdateVersionReaderRoleBinding(ctx, clientset, liqoNamespace); err != nil {
		return fmt.Errorf("failed to create/update version reader RoleBinding: %w", err)
	}

	klog.Infof("Successfully set up version resources (version: %s) in namespace %s", version, liqoNamespace)
	return nil
}

// createOrUpdateVersionConfigMap creates or updates the ConfigMap containing the Liqo version.
func createOrUpdateVersionConfigMap(ctx context.Context, clientset kubernetes.Interface, liqoNamespace, version string) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LiqoVersionConfigMapName,
			Namespace: liqoNamespace,
		},
		Data: map[string]string{
			LiqoVersionKey: version,
		},
	}

	resource.AddGlobalLabels(configMap)

	_, err := clientset.CoreV1().ConfigMaps(liqoNamespace).Get(ctx, LiqoVersionConfigMapName, metav1.GetOptions{})
	if err != nil {
		// ConfigMap doesn't exist, create it
		_, err = clientset.CoreV1().ConfigMaps(liqoNamespace).Create(ctx, configMap, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create ConfigMap %s/%s: %w", liqoNamespace, LiqoVersionConfigMapName, err)
		}
		klog.Infof("Created ConfigMap %s/%s with version %s", liqoNamespace, LiqoVersionConfigMapName, version)
	} else {
		// ConfigMap exists, update it
		_, err = clientset.CoreV1().ConfigMaps(liqoNamespace).Update(ctx, configMap, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ConfigMap %s/%s: %w", liqoNamespace, LiqoVersionConfigMapName, err)
		}
		klog.Infof("Updated ConfigMap %s/%s with version %s", liqoNamespace, LiqoVersionConfigMapName, version)
	}

	return nil
}

// createOrUpdateVersionReaderRole creates or updates the Role that allows reading the liqo-version ConfigMap.
func createOrUpdateVersionReaderRole(ctx context.Context, clientset kubernetes.Interface, liqoNamespace string) error {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LiqoVersionReaderRoleName,
			Namespace: liqoNamespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{""},
				Resources:     []string{"configmaps"},
				ResourceNames: []string{LiqoVersionConfigMapName},
				Verbs:         []string{"get"},
			},
		},
	}

	resource.AddGlobalLabels(role)

	_, err := clientset.RbacV1().Roles(liqoNamespace).Get(ctx, LiqoVersionReaderRoleName, metav1.GetOptions{})
	if err != nil {
		// Role doesn't exist, create it
		_, err = clientset.RbacV1().Roles(liqoNamespace).Create(ctx, role, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create Role %s/%s: %w", liqoNamespace, LiqoVersionReaderRoleName, err)
		}
		klog.Infof("Created Role %s/%s", liqoNamespace, LiqoVersionReaderRoleName)
	} else {
		// Role exists, update it
		_, err = clientset.RbacV1().Roles(liqoNamespace).Update(ctx, role, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update Role %s/%s: %w", liqoNamespace, LiqoVersionReaderRoleName, err)
		}
		klog.V(6).Infof("Updated Role %s/%s", liqoNamespace, LiqoVersionReaderRoleName)
	}

	return nil
}

// createOrUpdateVersionReaderRoleBinding creates or updates the RoleBinding for the liqo-version-reader Role.
func createOrUpdateVersionReaderRoleBinding(ctx context.Context, clientset kubernetes.Interface, liqoNamespace string) error {
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LiqoVersionReaderRoleBindingName,
			Namespace: liqoNamespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "Group",
				Name:     LiqoGroupName,
				APIGroup: "rbac.authorization.k8s.io",
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     LiqoVersionReaderRoleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	resource.AddGlobalLabels(roleBinding)

	_, err := clientset.RbacV1().RoleBindings(liqoNamespace).Get(ctx, LiqoVersionReaderRoleBindingName, metav1.GetOptions{})
	if err != nil {
		// RoleBinding doesn't exist, create it
		_, err = clientset.RbacV1().RoleBindings(liqoNamespace).Create(ctx, roleBinding, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create RoleBinding %s/%s: %w", liqoNamespace, LiqoVersionReaderRoleBindingName, err)
		}
		klog.Infof("Created RoleBinding %s/%s for group %s", liqoNamespace, LiqoVersionReaderRoleBindingName, LiqoGroupName)
	} else {
		// RoleBinding exists, update it
		_, err = clientset.RbacV1().RoleBindings(liqoNamespace).Update(ctx, roleBinding, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update RoleBinding %s/%s: %w", liqoNamespace, LiqoVersionReaderRoleBindingName, err)
		}
		klog.V(6).Infof("Updated RoleBinding %s/%s", liqoNamespace, LiqoVersionReaderRoleBindingName)
	}

	return nil
}

// GetRemoteVersion retrieves the Liqo version from a remote cluster using the provided clientset.
// It returns an empty string if the ConfigMap doesn't exist or if there's an error.
func GetRemoteVersion(ctx context.Context, remoteClientset kubernetes.Interface, liqoNamespace string) string {
	configMap, err := remoteClientset.CoreV1().ConfigMaps(liqoNamespace).Get(ctx, LiqoVersionConfigMapName, metav1.GetOptions{})
	if err != nil {
		klog.V(4).Infof("Failed to get remote version ConfigMap: %v", err)
		return ""
	}

	version, found := configMap.Data[LiqoVersionKey]
	if !found {
		klog.V(4).Infof("Version key not found in remote ConfigMap")
		return ""
	}

	return version
}

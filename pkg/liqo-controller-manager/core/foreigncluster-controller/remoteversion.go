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

package foreignclustercontroller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	authv1beta1 "github.com/liqotech/liqo/apis/authentication/v1beta1"
	liqov1beta1 "github.com/liqotech/liqo/apis/core/v1beta1"
	versionpkg "github.com/liqotech/liqo/pkg/liqo-controller-manager/version"
	"github.com/liqotech/liqo/pkg/utils/getters"
)

// handleRemoteVersion attempts to fetch the remote cluster's Liqo version
// and update it in the ForeignCluster status.
// It uses a hybrid approach:
// 1. If credentials are available (consumer role), fetch version from remote ConfigMap
// 2. If credentials are not available (provider role), read version from local Tenant resource
func (r *ForeignClusterReconciler) handleRemoteVersion(ctx context.Context, fc *liqov1beta1.ForeignCluster) {
	clusterID := fc.Spec.ClusterID
	var remoteVersion string

	// Method 1: Try to fetch version using credentials (when we have access to the remote cluster)
	if r.IdentityManager != nil {
		config, err := r.IdentityManager.GetConfig(clusterID, corev1.NamespaceAll)
		if err == nil {
			// We have credentials, fetch the version from the remote cluster's ConfigMap
			remoteClientset, err := kubernetes.NewForConfig(config)
			if err == nil {
				remoteVersion = versionpkg.GetRemoteVersion(ctx, remoteClientset, r.LiqoNamespace)
				if remoteVersion != "" {
					klog.V(4).Infof("Fetched remote version from ConfigMap for ForeignCluster %q: %s", clusterID, remoteVersion)
				}
			} else {
				klog.V(4).Infof("Failed to create clientset for remote cluster %q: %v", clusterID, err)
			}
		} else {
			klog.V(6).Infof("Unable to get config for remote cluster %q, will try Tenant fallback: %v", clusterID, err)
		}
	}

	// Method 2: Fallback - try to read version from local Tenant resource
	// This works when we are the provider and don't have credentials to access the consumer
	if remoteVersion == "" {
		remoteVersion = r.getVersionFromTenant(ctx, clusterID)
		if remoteVersion != "" {
			klog.V(4).Infof("Fetched remote version from Tenant for ForeignCluster %q: %s", clusterID, remoteVersion)
		}
	}

	// Update the ForeignCluster status if the version changed
	if remoteVersion != "" && remoteVersion != fc.Status.RemoteVersion {
		klog.Infof("Updated remote version for ForeignCluster %q: %s", clusterID, remoteVersion)
	}

	fc.Status.RemoteVersion = remoteVersion
}

// getVersionFromTenant attempts to read the Liqo version from the local Tenant resource
// created by the remote cluster. Returns empty string if Tenant is not found or has no version.
func (r *ForeignClusterReconciler) getVersionFromTenant(ctx context.Context, clusterID liqov1beta1.ClusterID) string {
	// Try to find the Tenant resource for this cluster ID in the Liqo namespace
	tenant, err := getters.GetTenantByClusterID(ctx, r.Client, clusterID, r.LiqoNamespace)
	if err != nil {
		klog.V(6).Infof("Unable to get Tenant for cluster %q: %v", clusterID, err)
		return ""
	}

	return tenant.Spec.LiqoVersion
}

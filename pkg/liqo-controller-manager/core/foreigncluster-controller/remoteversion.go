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

	liqov1beta1 "github.com/liqotech/liqo/apis/core/v1beta1"
	versionpkg "github.com/liqotech/liqo/pkg/liqo-controller-manager/version"
)

// handleRemoteVersion attempts to fetch the remote cluster's Liqo version
// and update it in the ForeignCluster status.
func (r *ForeignClusterReconciler) handleRemoteVersion(ctx context.Context, fc *liqov1beta1.ForeignCluster) {
	if r.IdentityManager == nil {
		klog.V(6).Infof("IdentityManager not available, skipping remote version fetch for ForeignCluster %q", fc.Name)
		return
	}

	clusterID := fc.Spec.ClusterID

	// Try to get a config for the remote cluster using the identity manager.
	// We use corev1.NamespaceAll to search across all tenant namespaces.
	config, err := r.IdentityManager.GetConfig(clusterID, corev1.NamespaceAll)
	if err != nil {
		// If we can't get a config, it means we don't have credentials for this cluster yet.
		// This is expected during the initial peering phase, so we log at a lower level.
		klog.V(6).Infof("Unable to get config for remote cluster %q: %v", clusterID, err)
		fc.Status.RemoteVersion = ""
		return
	}

	// Create a clientset to access the remote cluster.
	remoteClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.V(4).Infof("Failed to create clientset for remote cluster %q: %v", clusterID, err)
		fc.Status.RemoteVersion = ""
		return
	}

	// Fetch the remote version.
	remoteVersion := versionpkg.GetRemoteVersion(ctx, remoteClientset, r.LiqoNamespace)
	if remoteVersion != "" && remoteVersion != fc.Status.RemoteVersion {
		klog.Infof("Updated remote version for ForeignCluster %q: %s", clusterID, remoteVersion)
	}

	fc.Status.RemoteVersion = remoteVersion
}

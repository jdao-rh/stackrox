// Code generated by genny. DO NOT EDIT.
// This file was automatically generated by genny.
// Any changes will be lost if this file is regenerated.
// see https://github.com/mauricelam/genny

package fixtures

import (
	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/pkg/sac/testconsts"
	"github.com/stackrox/rox/pkg/uuid"
)

// *storage.ProcessIndicator represents a generic type that we use in the function below.

// GetSACTestStorageProcessIndicatorSet returns a set of mock *storage.ProcessIndicator that can be used
// for scoped access control sets.
// It will include:
// 9 *storage.ProcessIndicator scoped to Cluster1, 3 to each Namespace A / B / C.
// 9 *storage.ProcessIndicator scoped to Cluster2, 3 to each Namespace A / B / C.
// 9 *storage.ProcessIndicator scoped to Cluster3, 3 to each Namespace A / B / C.
func GetSACTestStorageProcessIndicatorSet(scopedStorageProcessIndicatorCreator func(id string, clusterID string, namespace string) *storage.ProcessIndicator) []*storage.ProcessIndicator {
	clusters := []string{testconsts.Cluster1, testconsts.Cluster2, testconsts.Cluster3}
	namespaces := []string{testconsts.NamespaceA, testconsts.NamespaceB, testconsts.NamespaceC}
	const numberOfAccounts = 3
	storageProcessIndicators := make([]*storage.ProcessIndicator, 0, len(clusters)*len(namespaces)*numberOfAccounts)
	for _, cluster := range clusters {
		for _, namespace := range namespaces {
			for i := 0; i < numberOfAccounts; i++ {
				storageProcessIndicators = append(storageProcessIndicators, scopedStorageProcessIndicatorCreator(uuid.NewV4().String(), cluster, namespace))
			}
		}
	}
	return storageProcessIndicators
}

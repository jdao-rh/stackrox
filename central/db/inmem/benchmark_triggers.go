package inmem

import (
	"sort"

	"bitbucket.org/stack-rox/apollo/central/db"
	"bitbucket.org/stack-rox/apollo/generated/api/v1"
	"bitbucket.org/stack-rox/apollo/pkg/protoconv"
	"bitbucket.org/stack-rox/apollo/pkg/set"
)

type benchmarkTriggerStore struct {
	db.BenchmarkTriggerStorage
}

func newBenchmarkTriggerStore(persistent db.BenchmarkTriggerStorage) *benchmarkTriggerStore {
	return &benchmarkTriggerStore{
		BenchmarkTriggerStorage: persistent,
	}
}

// GetBenchmarkTriggers returns a slice of triggers based on the request
func (s *benchmarkTriggerStore) GetBenchmarkTriggers(request *v1.GetBenchmarkTriggersRequest) ([]*v1.BenchmarkTrigger, error) {
	triggers, err := s.BenchmarkTriggerStorage.GetBenchmarkTriggers(request)
	if err != nil {
		return nil, err
	}
	idSet := set.NewSetFromStringSlice(request.GetIds())
	clusterSet := set.NewSetFromStringSlice(request.GetClusterIds())
	filteredTriggers := triggers[:0]
	for _, trigger := range triggers {
		if idSet.Cardinality() > 0 && !idSet.Contains(trigger.GetId()) {
			continue
		}
		// If request clusters is empty then return all
		// If the trigger has no cluster set, then it applies to all clusters
		if clusterSet.Cardinality() != 0 && len(trigger.ClusterIds) != 0 {
			var clusterMatch bool
			for _, cluster := range trigger.ClusterIds {
				if clusterSet.Contains(cluster) {
					clusterMatch = true
					break
				}
			}
			if !clusterMatch {
				continue
			}
		}

		// Check from_time <-> end_time
		// If FromTime is after trigger time then filter out
		if request.FromTime != nil && protoconv.CompareProtoTimestamps(request.FromTime, trigger.Time) == 1 {
			continue
		}
		// If the ToTime is less than the trigger time, then filter out
		if request.ToTime != nil && protoconv.CompareProtoTimestamps(request.ToTime, trigger.Time) == -1 {
			continue
		}
		filteredTriggers = append(filteredTriggers, trigger)
	}
	sort.SliceStable(filteredTriggers, func(i, j int) bool {
		return protoconv.CompareProtoTimestamps(filteredTriggers[i].Time, filteredTriggers[j].Time) == 1
	})
	return filteredTriggers, nil
}

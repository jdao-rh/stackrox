// Code generated by notifier generator. DO NOT EDIT.

package notifier

import (
	storage "github.com/stackrox/rox/generated/storage"
	sync "github.com/stackrox/rox/pkg/sync"
)

func newNotifier() *notifier {
	return &notifier{}
}

type notifier struct {
	lock      sync.RWMutex
	onAdds    []func(alert *storage.Alert)
	onDeletes []func(alert *storage.Alert)
	onUpdates []func(alert *storage.Alert)
}

func (n *notifier) OnAdd(onAdd func(alert *storage.Alert)) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.onAdds = append(n.onAdds, onAdd)
}

func (n *notifier) Added(alert *storage.Alert) {
	n.lock.RLock()
	defer n.lock.RUnlock()
	for _, f := range n.onAdds {
		f(alert)
	}
}

func (n *notifier) OnDelete(onDelete func(alert *storage.Alert)) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.onDeletes = append(n.onDeletes, onDelete)
}

func (n *notifier) Deleted(alert *storage.Alert) {
	n.lock.RLock()
	defer n.lock.RUnlock()
	for _, f := range n.onDeletes {
		f(alert)
	}
}

func (n *notifier) OnUpdate(onUpdate func(alert *storage.Alert)) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.onUpdates = append(n.onUpdates, onUpdate)
}

func (n *notifier) Updated(alert *storage.Alert) {
	n.lock.RLock()
	defer n.lock.RUnlock()
	for _, f := range n.onUpdates {
		f(alert)
	}
}

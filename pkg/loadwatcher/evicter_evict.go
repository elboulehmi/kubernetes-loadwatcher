package loadwatcher

import (
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"time"
)

func (e *Evicter) CanEvict() bool {
	if e.lastEviction.IsZero() {
		return true
	}

	return time.Now().Sub(e.lastEviction) > e.backoff
}

func (e *Evicter) EvictPod(evt LoadThresholdEvent) (bool, error) {
	if evt.Load15 < e.threshold {
		return false, nil
	}

	if !e.CanEvict() {
		glog.Infof("eviction threshold exceeded; still in back-off")
		return false, nil
	}

	glog.Infof("searching for pod to evict")

	fieldSelector := fields.OneTermEqualSelector("spec.nodeName", e.nodeName)

	podsOnNode, err := e.client.CoreV1().Pods("").List(metav1.ListOptions{
		FieldSelector: fieldSelector.String(),
	})

	if err != nil {
		return false, err
	}

	candidates := PodCandidateSetFromPodList(podsOnNode)
	podToEvict := candidates.SelectPodForEviction()

	if podToEvict == nil {
		e.recorder.Eventf(e.nodeRef, v1.EventTypeWarning, "NoPodToEvict", "wanted to evict Pod, but no suitable candidate found")
		return false, nil
	}

	eviction := v1beta1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name: podToEvict.ObjectMeta.Name,
			Namespace: podToEvict.ObjectMeta.Namespace,
		},
	}

	glog.Infof("eviction: %+v", eviction)

	e.lastEviction = time.Now()

	e.recorder.Eventf(podToEvict, v1.EventTypeWarning, "EvictHighLoad", "evicting pod due to high load on node load15=%.2f threshold=%.2f", evt.Load15, evt.LoadThreshold)
	e.recorder.Eventf(e.nodeRef, v1.EventTypeWarning, "EvictHighLoad", "evicting pod due to high load on node load15=%.2f threshold=%.2f", evt.Load15, evt.LoadThreshold)

	err = e.client.CoreV1().Pods(podToEvict.Namespace).Evict(&eviction)
	return true, err
}

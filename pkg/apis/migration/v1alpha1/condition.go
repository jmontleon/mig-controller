package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

// Condition
type Condition struct {
	Type               string      `json:"type"`
	Status             string      `json:"status"`
	Reason             string      `json:"reason,omitempty"`
	Message            string      `json:"message,omitempty"`
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
}

// Update this condition with another's fields.
func (r *Condition) Update(other Condition) {
	if r.Equal(other) {
		return
	}
	r.Type = other.Type
	r.Status = other.Status
	r.Reason = other.Reason
	r.Message = other.Message
	r.LastTransitionTime = metav1.NewTime(time.Now())
}

func (r Condition) Equal(other Condition) bool {
	return r.Type == other.Type &&
		r.Status == other.Status &&
		r.Reason == other.Reason &&
		r.Message == other.Message
}

// Managed collection of conditions.
// Intended to be included in resource Status.
type Conditions struct {
	Conditions []Condition `json:"conditions"`
}

func (r *Conditions) FindCondition(cndType string) (int, *Condition) {
	if r.Conditions == nil {
		return 0, nil
	}
	for index := range r.Conditions {
		condition := &r.Conditions[index]
		if condition.Type == cndType {
			return index, condition
		}
	}
	return 0, nil
}

// Set (add/update) the specified condition to the collection.
func (r *Conditions) SetCondition(condition Condition) {
	if r.Conditions == nil {
		r.Conditions = []Condition{}
	}
	_, found := r.FindCondition(condition.Type)
	if found == nil {
		condition.LastTransitionTime = metav1.NewTime(time.Now())
		r.Conditions = append(r.Conditions, condition)
	} else {
		found.Update(condition)
	}
}

// Delete conditions by type.
func (r *Conditions) DeleteCondition(cndTypes ...string) {
	if r.Conditions == nil {
		return
	}
	for _, name := range cndTypes {
		i, condition := r.FindCondition(name)
		if condition != nil {
			r.Conditions = append(r.Conditions[:i], r.Conditions[i+1:]...)
		}
	}
}

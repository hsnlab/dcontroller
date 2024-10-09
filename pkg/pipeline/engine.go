package pipeline

import (
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"hsnlab/dcontroller/pkg/cache"
	"hsnlab/dcontroller/pkg/object"
)

type gvk = schema.GroupVersionKind

type Engine interface {
	// EvaluateJoin evaluates a join expression.
	EvaluateJoin(j *Join, delta cache.Delta) ([]cache.Delta, error)
	// EvaluateAggregation evaluates an aggregation pipeline.
	EvaluateAggregation(a *Aggregation, delta cache.Delta) ([]cache.Delta, error)
	// IsValidEvent returns false for some invalid events, like null-events or duplicate
	// events.
	IsValidEvent(cache.Delta) bool
	// View returns the target view of the engine.
	View() string
	// WithObjects sets some base objects in the cache for testing.
	WithObjects(objects ...object.Object)
	// Log returns a logger.
	Log() logr.Logger
}

func Normalize(eng Engine, content unstruct) (object.Object, error) {
	// Normalize always produces Views!
	obj := object.NewViewObject(eng.View())

	// metadata: must exist
	meta, ok := content["metadata"]
	if !ok {
		return nil, NewInvalidObjectError("no metadata in object")
	}
	metaMap, ok := meta.(unstruct)
	if !ok {
		return nil, NewInvalidObjectError("invalid metadata in object")
	}

	// name must be defined
	name, ok := metaMap["name"]
	if !ok {
		return nil, NewInvalidObjectError("missing /metadata/name")
	}
	if reflect.ValueOf(name).Kind() != reflect.String {
		return nil, NewInvalidObjectError(fmt.Sprintf("metadata/name must be a string "+
			"(current value %q)", name))
	}
	nameStr := name.(string)
	if nameStr == "" {
		return nil, NewInvalidObjectError("empty metadata/name in aggregation result")
	}
	obj.SetName(nameStr)

	// namespace: can be empty
	namespace, ok := metaMap["namespace"]
	if !ok {
		obj.SetNamespace("")
	} else {
		if reflect.ValueOf(namespace).Kind() != reflect.String {
			return nil, NewInvalidObjectError(fmt.Sprintf("metadata/namespace must be "+
				"a string (current value %q)", namespace))
		}
		namespaceStr := namespace.(string)
		obj.SetNamespace(namespaceStr)
	}

	object.SetContent(obj, content)

	return obj, nil
}
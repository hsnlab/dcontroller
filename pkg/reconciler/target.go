package reconciler

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runtimeManager "sigs.k8s.io/controller-runtime/pkg/manager"

	opv1a1 "github.com/hsnlab/dcontroller/pkg/api/operator/v1alpha1"
	"github.com/hsnlab/dcontroller/pkg/cache"
	"github.com/hsnlab/dcontroller/pkg/object"
	"github.com/hsnlab/dcontroller/pkg/util"
)

// Target is a generic writer that knows how to create controller runtime objects in a target resource.
type Target interface {
	Resource
	Write(context.Context, cache.Delta) error
	fmt.Stringer
}

type target struct {
	Resource
	mgr    runtimeManager.Manager
	target opv1a1.Target
	log    logr.Logger
}

func NewTarget(mgr runtimeManager.Manager, t opv1a1.Target) Target {
	target := &target{
		Resource: NewResource(mgr, t.Resource),
		mgr:      mgr,
		target:   t,
	}

	log := mgr.GetLogger().WithName("target").WithValues("name", target.Resource.String())
	target.log = log

	return target
}

func (t *target) String() string {
	return fmt.Sprintf("%s<type:%s>", t.Resource.String(), t.target.Type)
}

// Write enforces a delta on a target. The behavior depends on the target type:
//   - For Updaters the delta is enforced as is to the target
//   - For Patchers the delta object is applied as a strategic merge patch: for Add and Update
//     deltas the target is patched with the delta object, while for Delete the delta object
//     content is removed from the target using a strategic merge patch.
func (t *target) Write(ctx context.Context, delta cache.Delta) error {
	if delta.Object == nil {
		return errors.New("write: empty object in delta")
	}

	// gvk to watch
	gvk, err := t.GetGVK()
	if err != nil {
		return err
	}

	// make a private copy of the Object
	delta.Object = object.DeepCopy(delta.Object)

	// make sure delta object gets the correct GVK applied
	delta.Object.SetGroupVersionKind(gvk)

	switch t.target.Type {
	case opv1a1.Updater, "":
		return t.update(ctx, delta)
	case opv1a1.Patcher:
		return t.patch(ctx, delta)
	default:
		return fmt.Errorf("unknown target type: %s", t.target.Type)
	}
}

func (t *target) update(ctx context.Context, delta cache.Delta) error {
	t.log.V(5).Info("updating target", "delta-type", delta.Type, "object", object.Dump(delta.Object))

	c := t.mgr.GetClient()

	//nolint:nolintlint
	switch delta.Type { //nolint:exhaustive
	case cache.Added, cache.Upserted, cache.Updated, cache.Replaced:
		t.log.V(2).Info("add/upsert", "event-type", delta.Type, "object", client.ObjectKeyFromObject(delta.Object))

		gvk, err := t.Resource.GetGVK()
		if err != nil {
			return err
		}
		obj := object.New()
		obj.SetGroupVersionKind(gvk)
		obj.SetName(delta.Object.GetName())
		obj.SetNamespace(delta.Object.GetNamespace())

		// WARNING: the Update target cannot be used to delete labels and annotations, use
		// the Patcher target for that (this is because we don't want the user to remove
		// important labels/annotations accidentally and taking care of each in the
		// pipeline may be too difficult)
		//
		// Use our own CreateOrUpdate that will also update the status
		res, err := CreateOrUpdate(context.TODO(), c, obj, func() error {
			// remove stuff that's no longer there
			for k := range obj.UnstructuredContent() {
				if k == "metadata" {
					continue
				}
				if _, ok, _ := unstructured.NestedFieldNoCopy(delta.Object.UnstructuredContent(), k); !ok {
					unstructured.RemoveNestedField(obj.UnstructuredContent(), k)
				}
			}

			// then update the content with new keys: metadata and status will be handled separately
			for k, v := range delta.Object.UnstructuredContent() {
				if k == "metadata" {
					continue
				}

				if err := unstructured.SetNestedField(obj.UnstructuredContent(), v, k); err != nil {
					t.log.Error(err, "failed to update object field during update",
						"object", client.ObjectKeyFromObject(obj).String(), "key", k)
					continue
				}
			}

			mergeMetadata(obj, delta.Object)

			// restore metadata
			obj.SetGroupVersionKind(gvk)
			obj.SetName(delta.Object.GetName())
			obj.SetNamespace(delta.Object.GetNamespace())

			return nil
		})

		if err != nil {
			return fmt.Errorf("create/update resource %s failed with operation code %s: %w",
				client.ObjectKeyFromObject(delta.Object).String(), res, err)
		}

		return nil

	case cache.Deleted:
		t.log.V(2).Info("delete", "event-type", delta.Type, "object", client.ObjectKeyFromObject(delta.Object))

		return c.Delete(ctx, delta.Object)

	default:
		t.log.V(3).Info("target: ignoring delta", "type", delta.Type)

		return nil
	}
}

func (t *target) patch(ctx context.Context, delta cache.Delta) error {
	t.log.V(5).Info("patching target", "delta-type", delta.Type, "object", object.Dump(delta.Object))

	c := t.mgr.GetClient()

	//nolint:nolintlint
	switch delta.Type { //nolint:exhaustive
	case cache.Added, cache.Updated, cache.Upserted, cache.Replaced:
		t.log.V(4).Info("update-patch", "event-type", delta.Type,
			"key", client.ObjectKeyFromObject(delta.Object).String())

		obj := object.New()
		obj.SetGroupVersionKind(delta.Object.GroupVersionKind())
		obj.SetName(delta.Object.GetName())
		obj.SetNamespace(delta.Object.GetNamespace())
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			return err
		}

		// TODO: strategic merge patch fails with error "unable to find api field in struct
		// Unstructured for the json field \"metadata\""}"
		// return c.Patch(ctx, result, client.RawPatch(types.StrategicMergePatchType, patch))

		// fall back to simple merge patches
		patch, err := json.Marshal(object.DeepCopy(delta.Object).UnstructuredContent())
		if err != nil {
			return err
		}
		if err := c.Patch(ctx, obj, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return err
		}

		// Patch does not update the status so we have to do this separately
		// must copy status here otherwise Patch may reewrite it
		newStatus, hasStatus, _ := unstructured.NestedMap(delta.Object.UnstructuredContent(), "status")
		if hasStatus {
			if err := unstructured.SetNestedMap(obj.Object, newStatus, "status"); err != nil {
				return err
			}

			patch, err = json.Marshal(obj)
			if err != nil {
				return err
			}

			return c.Status().Patch(ctx, obj, client.RawPatch(types.MergePatchType, patch))
		}

		return nil

	case cache.Deleted:
		// apply the patch locally so that we fully control the behavior
		patch := removeNested(delta.Object.UnstructuredContent())

		// make sure we do not remove crucial metadata: the GVK and the namespace/name
		gvk := delta.Object.GroupVersionKind()
		gr := schema.GroupVersion{Group: gvk.Group, Version: gvk.Version}
		unstructured.SetNestedField(patch, gr.String(), "apiVersion")                            //nolint:errcheck
		unstructured.SetNestedField(patch, gvk.Kind, "kind")                                     //nolint:errcheck
		unstructured.SetNestedField(patch, delta.Object.GetNamespace(), "metadata", "namespace") //nolint:errcheck
		unstructured.SetNestedField(patch, delta.Object.GetName(), "metadata", "name")           //nolint:errcheck

		b, err := json.Marshal(patch)
		if err != nil {
			return err
		}

		t.log.V(4).Info("delete-patch", "event-type", delta.Type,
			"object", client.ObjectKeyFromObject(delta.Object),
			"patch", util.Stringify(patch), "raw-patch", string(b))

		if err := c.Patch(context.Background(), delta.Object, client.RawPatch(types.StrategicMergePatchType, b)); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
		}

		return nil

	default:
		t.log.V(2).Info("target: ignoring delta", "type", delta.Type)

		return nil
	}
}

func removeNested(m map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		if nestedMap, ok := v.(map[string]any); ok {
			result[k] = removeNested(nestedMap)
		} else if nestedSlice, ok := v.([]any); ok {
			// TODO: handle nested slices!!!!
			result[k] = nestedSlice
		} else {
			result[k] = nil
		}
	}
	return result
}

func mergeMetadata(obj, new object.Object) {
	labels := obj.GetLabels()
	newLabels := new.GetLabels()
	if newLabels != nil {
		if labels == nil {
			labels = map[string]string{}
		}
		for k, v := range newLabels {
			labels[k] = v
		}
		obj.SetLabels(labels)
	}

	annotations := obj.GetAnnotations()
	newAnnotations := new.GetAnnotations()
	if newAnnotations != nil {
		if annotations == nil {
			annotations = map[string]string{}
		}
		for k, v := range newAnnotations {
			annotations[k] = v
		}
		obj.SetAnnotations(annotations)
	}
}

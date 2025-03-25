package reconciler

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeManager "sigs.k8s.io/controller-runtime/pkg/manager"

	opv1a1 "github.com/hsnlab/dcontroller/pkg/api/operator/v1alpha1"
	viewv1a1 "github.com/hsnlab/dcontroller/pkg/api/view/v1alpha1"
	"github.com/hsnlab/dcontroller/pkg/util"
)

type Resource interface {
	fmt.Stringer
	GetGVK() (schema.GroupVersionKind, error)
}

type resource struct {
	mgr      runtimeManager.Manager
	resource opv1a1.Resource
}

func NewResource(mgr runtimeManager.Manager, r opv1a1.Resource) Resource {
	return &resource{
		mgr:      mgr,
		resource: r,
	}
}

func (r *resource) String() string {
	gvk, err := r.GetGVK()
	if err != nil {
		return ""
	}
	// do not use the standard notation: it adds spaces
	gr := gvk.Group
	if gr == "" {
		gr = "core"
	}
	return fmt.Sprintf("%s/%s:%s", gr, gvk.Version, gvk.Kind)
}

func (r *resource) GetGVK() (schema.GroupVersionKind, error) {
	if r.resource.Kind == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("empty Kind in %s", util.Stringify(*r))
	}

	if r.resource.Group == nil || *r.resource.Group == viewv1a1.GroupVersion.Group {
		// this will be a View, version is enforced
		return r.getGVKByGroupKind(schema.GroupKind{Group: viewv1a1.GroupVersion.Group, Kind: r.resource.Kind})
	}

	// this will be a standard Kubernetes object
	if r.resource.Version == nil {
		return r.getGVKByGroupKind(schema.GroupKind{Group: *r.resource.Group, Kind: r.resource.Kind})
	}
	return schema.GroupVersionKind{
		Group:   *r.resource.Group,
		Version: *r.resource.Version,
		Kind:    r.resource.Kind,
	}, nil
}

func (r *resource) getGVKByGroupKind(gr schema.GroupKind) (schema.GroupVersionKind, error) {
	if gr.Group == viewv1a1.GroupVersion.Group {
		return schema.GroupVersionKind{
			Group:   viewv1a1.GroupVersion.Group,
			Kind:    gr.Kind,
			Version: viewv1a1.GroupVersion.Version,
		}, nil
	}

	// standard Kubernetes object
	mapper := r.mgr.GetRESTMapper()
	gvk, err := mapper.KindFor(schema.GroupVersionResource{Group: gr.Group, Resource: gr.Kind})
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("cannot find GVK for %s: %w", gr, err)
	}

	return gvk, nil
}

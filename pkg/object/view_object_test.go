package object

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "hsnlab/dcontroller-runtime/pkg/api/view/v1"
)

var _ client.Object = &ViewObject{}
var _ schema.ObjectKind = &ViewObject{}
var _ metav1.ListInterface = &ViewObject{}
var _ Unstructured = &ViewObject{}

var _ = Describe("ViewObject", func() {
	It("should be created without content", func() {
		obj := NewViewObject("view")

		Expect(obj).NotTo(BeNil())
		Expect(obj.GetKind()).To(Equal("view"))
	})

	It("should implement metav1.Object", func() {
		obj := NewViewObject("view")
		Expect(obj).NotTo(BeNil())
		obj.SetNamespace("ns")
		obj.SetName("test-1")

		Expect(obj.GetKind()).To(Equal("view"))
		Expect(obj.GetNamespace()).To(Equal("ns"))
		Expect(obj.GetName()).To(Equal("test-1"))
		Expect(obj.UnstructuredContent()).To(Equal(map[string]any{
			"apiVersion": "view.dcontroller.github.io/v1alpha1",
			"kind":       "view",
			"metadata": map[string]any{
				"name":      "test-1",
				"namespace": "ns",
			},
		}))
	})

	It("should let GVK to be updated", func() {
		obj := NewViewObject("view")
		Expect(obj).NotTo(BeNil())

		obj.SetKind("dummy")
		Expect(obj.GetAPIVersion()).To(Equal(apiv1.GroupVersion.String()))
		Expect(obj.GetKind()).To(Equal("dummy"))

		obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "dummy-group", Version: "dummy-version", Kind: "dummy-kind"})
		Expect(obj.GetAPIVersion()).To(Equal(apiv1.GroupVersion.String()))
		Expect(obj.GetKind()).To(Equal("dummy-kind"))
	})

	It("should allow to be created with given namespace/name", func() {
		obj := NewViewObject("view").WithName("ns", "test-1")
		Expect(obj.GetNamespace()).To(Equal("ns"))
		Expect(obj.GetName()).To(Equal("test-1"))
	})

	It("should implement UnstructuredContent() in runtime.Unstructured", func() {
		obj := NewViewObject("view")

		Expect(obj).NotTo(BeNil())
		Expect(obj.GetKind()).To(Equal("view"))

		Expect(obj.UnstructuredContent()).To(Equal(map[string]any{
			"apiVersion": "view.dcontroller.github.io/v1alpha1",
			"kind":       "view",
		}))
	})

	It("should implement SetUnstructuredContent() in runtime.Unstructured", func() {
		obj := NewViewObject("view")

		Expect(obj).NotTo(BeNil())
		Expect(obj.GetKind()).To(Equal("view"))

		obj.SetUnstructuredContent(map[string]any{
			"apiVersion": "view.dcontroller.github.io/v1alpha1",
			"kind":       "different-view",
		})

		// view is readonly once the object has been created
		Expect(obj.UnstructuredContent()).To(Equal(map[string]any{
			"apiVersion": "view.dcontroller.github.io/v1alpha1",
			"kind":       "view",
		}))
	})

	It("should be created with content", func() {
		obj := NewViewObject("view").WithContent(map[string]any{"a": int64(1), "b": map[string]any{"c": int64(2)}})
		Expect(obj).NotTo(BeNil())
		obj.SetNamespace("ns")
		obj.SetName("test-1")

		Expect(obj.GetKind()).To(Equal("view"))
		Expect(obj.GetNamespace()).To(Equal("ns"))
		Expect(obj.GetName()).To(Equal("test-1"))
		Expect(obj.UnstructuredContent()).To(Equal(map[string]any{
			"apiVersion": "view.dcontroller.github.io/v1alpha1",
			"kind":       "view",
			"metadata": map[string]any{
				"name":      "test-1",
				"namespace": "ns",
			},
			"a": int64(1),
			"b": map[string]any{"c": int64(2)},
		}))
	})

	It("unstructured fields", func() {
		obj := NewViewObject("view")
		obj.SetUnstructuredContent(map[string]any{"a": int64(1)})

		val, ok, err := unstructured.NestedInt64(obj.Object, "a")
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		Expect(val).To(Equal(int64(1)))

		obj2 := obj.DeepCopy()
		val, ok, err = unstructured.NestedInt64(obj2.Object, "a")
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		Expect(val).To(Equal(int64(1)))
	})
})

func TestObject(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Object/ObjectList tests")
}
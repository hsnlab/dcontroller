package object

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ObjectList", func() {
	It("should allow a raw object to be created", func() {
		obj := New("view").WithName("ns", "test")
		list := &ObjectList{
			View:   "view",
			Object: map[string]interface{}{"kind": "viewlist", "apiVersion": "dcontroller.github.io/v1"},
			Items:  []Object{*obj},
		}
		content := list.UnstructuredContent()
		items := content["items"].([]interface{})
		Expect(items).To(HaveLen(1))

		item := items[0].(map[string]any)
		val, found, err := unstructured.NestedString(item, "metadata", "namespace")
		Expect(found).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal("ns"))

		val, found, err = unstructured.NestedString(item, "metadata", "name")
		Expect(found).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal("test"))
	})

	It("should allow to be created with the proper GVK", func() {
		list := NewList("view")

		Expect(list.GetAPIVersion()).To(Equal("dcontroller.github.io/v1alpha1"))
		Expect(list.GetKind()).To(Equal("view"))

		gvk := list.GroupVersionKind()
		Expect(gvk.Group).To(Equal("dcontroller.github.io"))
		Expect(gvk.Version).To(Equal("v1alpha1"))
		Expect(gvk.Kind).To(Equal("view"))
	})

	It("should allow to be created without content", func() {
		list := NewList("view")
		obj := New("view").WithName("ns", "test")
		list.Items = append(list.Items, *obj)

		content := list.UnstructuredContent()
		items := content["items"].([]interface{})
		Expect(items).To(HaveLen(1))

		item := items[0].(map[string]any)
		val, found, err := unstructured.NestedString(item, "metadata", "namespace")
		Expect(found).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal("ns"))

		val, found, err = unstructured.NestedString(item, "metadata", "name")
		Expect(found).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal("test"))
	})

	It("should be created with content", func() {
		obj := New("view").WithName("ns", "test").WithContent(map[string]any{"a": int64(1)})
		list := NewList("view")
		list.Items = append(list.Items, *obj)

		content := list.UnstructuredContent()
		items := content["items"].([]interface{})
		Expect(items).To(HaveLen(1))

		item := items[0].(map[string]any)
		val, found, err := unstructured.NestedString(item, "metadata", "namespace")
		Expect(found).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal("ns"))

		val, found, err = unstructured.NestedString(item, "metadata", "name")
		Expect(found).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal("test"))

		item = items[0].(map[string]any)
		i, found, err := unstructured.NestedInt64(item, "a")
		Expect(found).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())
		Expect(i).To(Equal(int64(1)))
	})

})
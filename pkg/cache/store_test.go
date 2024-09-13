package cache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"hsnlab/dcontroller-runtime/pkg/object"
)

var _ = Describe("Store", func() {
	var (
		store      *Store
		obj1, obj2 object.Object
	)

	BeforeEach(func() {
		store = NewStore()
		obj1 = object.NewViewObject("view").WithContent(map[string]any{"a": "x"}).WithName("ns", "name")
		obj2 = &unstructured.Unstructured{}
		obj2.SetUnstructuredContent(map[string]any{"a": "y"})
		obj2.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{Group: "testgroup", Version: "v1", Kind: "testkind"})
		obj2.SetNamespace("ns")
		obj2.SetName("name-1")
	})

	Describe("Add and Get operations", func() {
		It("should retrieve added objects", func() {
			err := store.Add(obj1)
			Expect(err).NotTo(HaveOccurred())

			retrieved, ok, err := store.Get(obj1)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(object.DeepEqual(retrieved, obj1)).To(BeTrue())

			err = store.Add(obj2)
			Expect(err).NotTo(HaveOccurred())

			retrieved, ok, err = store.Get(obj2)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(retrieved).To(Equal(obj2))
		})

		It("should return an error for non-existent object", func() {
			obj := object.NewViewObject("view").WithName("", "non-existent")
			_, ok, err := store.Get(obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})
	})

	Describe("List operation", func() {
		It("should list all added objects", func() {
			objects := []object.Object{
				object.NewViewObject("view").WithContent(map[string]any{"a": int64(1)}).WithName("ns1", "test-1"),
				object.NewViewObject("view").WithContent(map[string]any{"b": int64(2)}).WithName("ns2", "test-2"),
				object.NewViewObject("view").WithContent(map[string]any{"c": int64(3)}).WithName("ns3", "test-3"),
			}

			for _, obj := range objects {
				err := store.Add(obj)
				Expect(err).NotTo(HaveOccurred())
			}

			objs := store.List()
			Expect(objs).To(HaveLen(3))
			Expect(object.DeepEqual(objs[0], objects[0])).NotTo(BeNil())
			Expect(object.DeepEqual(objs[1], objects[1])).NotTo(BeNil())
			Expect(object.DeepEqual(objs[2], objects[2])).NotTo(BeNil())
		})

		It("should return an empty list when store is empty", func() {
			objs := store.List()
			Expect(objs).To(BeEmpty())
		})
	})
})
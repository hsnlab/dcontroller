package expression

import (
	"fmt"
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"

	"hsnlab/dcontroller/pkg/object"
)

var (
	loglevel = -10
	logger   = zap.New(zap.UseFlagOptions(&zap.Options{
		Development:     true,
		DestWriter:      GinkgoWriter,
		StacktraceLevel: zapcore.Level(3),
		TimeEncoder:     zapcore.RFC3339NanoTimeEncoder,
		Level:           zapcore.Level(loglevel),
	}))
)

func TestPipeline(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Expression")
}

var _ = Describe("Expressions", func() {
	var obj1, obj2 object.Object

	BeforeEach(func() {
		obj1 = object.NewViewObject("testview1")
		object.SetContent(obj1, Unstructured{
			"spec": Unstructured{
				"a": int64(1),
				"b": Unstructured{"c": int64(2)},
				"x": []any{int64(1), int64(2), int64(3), int64(4), int64(5)},
			},
		})
		object.SetName(obj1, "default", "name")

		obj2 = object.NewViewObject("testview2")
		object.SetContent(obj2, Unstructured{
			"metadata": Unstructured{
				"namespace": "default2",
				"name":      "name",
			},
			"spec": []any{
				Unstructured{
					"name": "name1",
					"a":    int64(1),
					"b":    Unstructured{"c": int64(2)},
				}, Unstructured{
					"name": "name2",
					"a":    int64(2),
					"b":    Unstructured{"d": int64(3)},
				},
			},
		})
	})

	Describe("Evaluating terminal expressions", func() {
		It("should deserialize and evaluate a bool literal expression", func() {
			jsonData := "true"
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{Op: "@bool", Literal: true}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Bool))
			Expect(reflect.ValueOf(res).Bool()).To(BeTrue())
		})

		It("should deserialize and evaluate an integer literal expression", func() {
			jsonData := "10"
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{Op: "@int", Literal: int64(10)}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Int64))
			Expect(reflect.ValueOf(res).Int()).To(Equal(int64(10)))
		})

		It("should deserialize and evaluate a float literal expression", func() {
			jsonData := "10.12"
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{Op: "@float", Literal: 10.12}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Float64))
			Expect(reflect.ValueOf(res).Float()).To(Equal(10.12))
		})

		It("should deserialize and evaluate a string literal expression", func() {
			jsonData := `"a10"`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{Op: "@string", Literal: "a10"}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.String))
			Expect(reflect.ValueOf(res).String()).To(Equal("a10"))
		})
	})

	Describe("Evaluating JSONPath expressions", func() {
		It("should evaluate a JSONPath expression on Kubernetes object", func() {
			input := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "testnamespace",
					Name:      "testservice-ok",
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeLoadBalancer,
					Selector: map[string]string{"app": "dummy"},
					Ports: []corev1.ServicePort{
						{
							Name:     "udp-ok",
							Protocol: corev1.ProtocolUDP,
							Port:     1,
						},
						{
							Name:     "tcp-ok",
							Protocol: corev1.ProtocolTCP,
							Port:     2,
						},
					},
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{{
							IP: "1.2.3.4",
							Ports: []corev1.PortStatus{{
								Port:     1,
								Protocol: corev1.ProtocolUDP,
							}, {
								Port:     2,
								Protocol: corev1.ProtocolTCP,
							}},
						}},
					},
				},
			}

			obj, err := object.NewViewObjectFromNativeObject("Service", input)
			Expect(err).NotTo(HaveOccurred())

			res, err := GetJSONPathExp(`$.metadata.name`, obj.UnstructuredContent())
			Expect(err).NotTo(HaveOccurred())
			s, err := AsString(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(Equal("testservice-ok"))

			res, err = GetJSONPathExp(`$["metadata"]["namespace"]`, obj.UnstructuredContent())
			Expect(err).NotTo(HaveOccurred())
			s, err = AsString(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(Equal("testnamespace"))

			res, err = GetJSONPathExp(`$.metadata`, obj.UnstructuredContent())
			Expect(err).NotTo(HaveOccurred())
			d, ok := res.(Unstructured)
			Expect(ok).To(BeTrue())
			Expect(d).To(HaveKey("namespace"))
			Expect(d["namespace"]).To(Equal("testnamespace"))
			Expect(d).To(HaveKey("name"))
			Expect(d["name"]).To(Equal("testservice-ok"))

			res, err = GetJSONPathExp(`$.spec.ports[1].port`, obj.UnstructuredContent())
			Expect(err).NotTo(HaveOccurred())
			i, err := AsInt(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(i).To(Equal(int64(2)))

			res, err = GetJSONPathExp(`$.spec.ports[?(@.name == 'udp-ok')].protocol`,
				obj.UnstructuredContent())
			Expect(err).NotTo(HaveOccurred())
			s, err = AsString(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(Equal("UDP"))
		})

		It("should evaluate an escaped JSONPath expression on Kubernetes object", func() {
			obj, err := object.NewViewObjectFromNativeObject("Service", &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "testnamespace",
					Name:      "testservice-ok",
				},
				Spec: corev1.ServiceSpec{},
			})
			Expect(err).NotTo(HaveOccurred())

			obj.SetAnnotations(map[string]string{
				"kubernetes.io/service-name":  "example", // actually used for enspointslices
				"kubernetes.io[service-name]": "weirdness",
			})

			// must use the alternative form
			res, err := GetJSONPathExp(`$["metadata"]["annotations"]["kubernetes.io/service-name"]`,
				obj.UnstructuredContent())
			Expect(err).NotTo(HaveOccurred())
			s, err := AsString(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(Equal("example"))

			res, err = GetJSONPathExp(`$["metadata"]["annotations"]["kubernetes.io[service-name]"]`,
				obj.UnstructuredContent())
			Expect(err).NotTo(HaveOccurred())
			s, err = AsString(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(Equal("weirdness"))
		})

		It("should evaluate a JSONPath expression", func() {
			jsonPath := "$.metadata.name"
			exp := Expression{Op: "@string", Literal: jsonPath}

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(res).To(Equal("name"))
		})

		It("should deserialize and evaluate an int JSONPath expression", func() {
			jsonData := `"$.spec.a"`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{Op: "@string", Literal: "$.spec.a"}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(res).To(Equal(int64(1)))
		})

		It("should deserialize and evaluate an alternative form of the int JSONPath expression", func() {
			jsonData := `"$[\"spec\"][\"a\"]"`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{Op: "@string", Literal: "$[\"spec\"][\"a\"]"}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(res).To(Equal(int64(1)))
		})

		It("should deserialize and evaluate a string JSONPath expression", func() {
			jsonData := `"$.metadata.namespace"`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{Op: "@string", Literal: "$.metadata.namespace"}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(res).To(Equal("default"))
		})

		It("should deserialize and evaluate a complex JSONPath expression", func() {
			jsonData := `"$.spec.b"`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{Op: "@string", Literal: "$.spec.b"}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(res).To(Equal(Unstructured{"c": int64(2)}))
		})

		It("should deserialize and evaluate a full JSONPath expression", func() {
			jsonData := `"$"`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{Op: "@string", Literal: "$"}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(res).To(Equal(Unstructured{
				"apiVersion": "view.dcontroller.io/v1alpha1",
				"kind":       "testview1",
				"metadata": Unstructured{
					"name":      "name",
					"namespace": "default",
				},
				"spec": Unstructured{
					"a": int64(1),
					"b": Unstructured{"c": int64(2)},
					"x": []any{int64(1), int64(2), int64(3), int64(4), int64(5)},
				},
			}))
		})

		It("should deserialize and evaluate a list search JSONPath expression with deref", func() {
			jsonData := `"$.spec[?(@.name == 'name1')].b"`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{
				Op:      "@string",
				Literal: "$.spec[?(@.name == 'name1')].b",
			}))

			ctx := EvalCtx{Object: obj2.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(res).To(Equal(Unstructured{"c": int64(2)}))
		})

		It("should deserialize and evaluate a list search JSONPath expression returning a list", func() {
			jsonData := `"$.spec[?(@.name == 'name2')]"`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			ctx := EvalCtx{Object: obj2.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(res).To(Equal(Unstructured{
				"name": "name2",
				"a":    int64(2),
				"b":    Unstructured{"d": int64(3)},
			}))
		})

		It("should deserialize and evaluate a list search JSONPath expression returning a list", func() {
			jsonData := `"$.spec[?(@.name in ['name1', 'name2'])].b.d"` // we return the last match
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj2.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(res).To(Equal(int64(3)))
		})

		It("should deserialize and evaluate a JSONPath expression used as a key in a map", func() {
			jsonData := `{"$.y[3]":12}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			d, ok := res.(Unstructured)
			Expect(ok).To(BeTrue())
			Expect(d).To(HaveKey("y"))
			Expect(d["y"]).To(Equal([]any{nil, nil, nil, int64(12)}))
		})

		It("should deserialize and evaluate a JSONPath expression used both as as a key and a value in a map", func() {
			jsonData := `{"$.y.z":"$.spec.b"}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			d, ok := res.(Unstructured)
			Expect(ok).To(BeTrue())
			Expect(d).To(HaveKey("y"))
			Expect(d["y"]).To(Equal(Unstructured{"z": Unstructured{"c": int64(2)}}))
		})

		It("should deserialize and evaluate a standard root ref JSONPath expression as a right value", func() {
			jsonData := `{"a":"$"}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			d, ok := res.(Unstructured)
			Expect(ok).To(BeTrue())
			Expect(d).To(HaveKey("a"))
			Expect(d["a"]).To(Equal(obj1.UnstructuredContent()))
		})

		It("should deserialize and evaluate a non-standard root ref JSONPath expression as a right value", func() {
			jsonData := `{"a":"$."}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			d, ok := res.(Unstructured)
			Expect(ok).To(BeTrue())
			Expect(d).To(HaveKey("a"))
			Expect(d["a"]).To(Equal(obj1.UnstructuredContent()))
		})

		It("should deserialize and evaluate a root ref JSONPath expression as a left value", func() {
			jsonData := `{"$.":{"a":"b"}}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			d, ok := res.(Unstructured)
			Expect(ok).To(BeTrue())
			Expect(d).To(Equal(map[string]any{"a": "b"}))
		})

		It("should err for a root ref JSONPath expression trying to copy a non-map right value", func() {
			jsonData := `{"$.":"a"}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			_, err = exp.Evaluate(ctx)
			Expect(err).To(HaveOccurred())
		})

		It("should deserialize and evaluate a JSONpath copy expression", func() {
			jsonData := `{"$.": {"a":1}}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			d, ok := res.(Unstructured)
			Expect(ok).To(BeTrue())
			Expect(d).To(Equal(map[string]any{"a": int64(1)}))
		})

		It("should deserialize and evaluate a setter with multiple JSONPath expressions", func() {
			jsonData := `{"$.spec.y":"aaa","$.spec.b.d":12}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			d, ok := res.(Unstructured)
			Expect(ok).To(BeTrue())

			Expect(d).To(HaveKey("spec"))
			Expect(d["spec"]).To(HaveKey("y"))
			Expect(d["spec"].(Unstructured)["y"]).To(Equal("aaa"))

			Expect(d["spec"]).To(HaveKey("b"))
			Expect(d["spec"].(Unstructured)["b"]).To(Equal(map[string]any{"d": int64(12)}))
		})

		It("should deserialize and evaluate a setter with multiple JSONPath expressions", func() {
			jsonData := `{"$.spec.y":"aaa","$.spec.b.c":"$.spec.b.c","$.spec.b.d":12}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			d, ok := res.(Unstructured)
			Expect(ok).To(BeTrue())

			Expect(d).To(HaveKey("spec"))
			Expect(d["spec"]).To(HaveKey("y"))
			Expect(d["spec"].(Unstructured)["y"]).To(Equal("aaa"))

			Expect(d["spec"]).To(HaveKey("b"))
			Expect(d["spec"].(Unstructured)["b"]).To(Equal(map[string]any{"c": int64(2), "d": int64(12)}))
		})
	})

	Describe("Evaluating compound expressions", func() {
		It("should deserialize and evaluate a nil expression", func() {
			jsonData := `{"@isnil": 1}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{
				Op:  "@isnil",
				Arg: &Expression{Op: "@int", Literal: int64(1)},
			}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Bool))
			Expect(reflect.ValueOf(res).Bool()).To(BeFalse())
		})

		It("should deserialize and evaluate an @exists expression", func() {
			jsonData := `{"@exists": "$.metadata.annotations.ann"}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{
				Op:  "@exists",
				Arg: &Expression{Op: "@string", Literal: "$.metadata.annotations.ann"},
			}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Bool))
			Expect(reflect.ValueOf(res).Bool()).To(BeFalse())
		})

		It("should deserialize and evaluate a bool expression", func() {
			jsonData := `{"@not": false}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{
				Op:  "@not",
				Arg: &Expression{Op: "@bool", Literal: false},
			}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Bool))
			Expect(reflect.ValueOf(res).Bool()).To(BeTrue())
		})

		It("should deserialize and evaluate a compound literal expression", func() {
			jsonData := `{"@eq": [10, 10]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{
				Op: "@eq",
				Arg: &Expression{
					Op: "@list",
					Literal: []Expression{
						{Op: "@int", Literal: int64(10)},
						{Op: "@int", Literal: int64(10)},
					},
				},
			}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Bool),
				fmt.Sprintf("kind mismatch: %v != %v",
					reflect.ValueOf(res).Kind(), reflect.Int64))
			Expect(reflect.ValueOf(res).Bool()).To(BeTrue())
		})

		It("should deserialize and err for a malformed expression", func() {
			jsonData := `{"@eq":10}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{
				Op: "@eq",
				Arg: &Expression{
					Op:      "@int",
					Literal: int64(10),
				},
			}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			_, err = exp.Evaluate(ctx)
			Expect(err).To(HaveOccurred())
		})

		It("should deserialize and evaluate a multi-level compound literal expression", func() {
			jsonData := `{"@and": [{"@eq": [10, 10]}, {"@lt": [1, 2]}]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{
				Op: "@and",
				Arg: &Expression{
					Op: "@list",
					Literal: []Expression{{
						Op: "@eq",
						Arg: &Expression{
							Op: "@list",
							Literal: []Expression{
								{Op: "@int", Literal: int64(10)},
								{Op: "@int", Literal: int64(10)},
							},
						},
					}, {
						Op: "@lt",
						Arg: &Expression{
							Op: "@list",
							Literal: []Expression{
								{Op: "@int", Literal: int64(1)},
								{Op: "@int", Literal: int64(2)},
							},
						},
					}},
				},
			}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Bool))
			Expect(reflect.ValueOf(res).Bool()).To(BeTrue())
		})

		It("should deserialize and evaluate a compound bool expression", func() {
			jsonData := `{"@bool":{"@eq":[10,10]}}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{
				Op: "@bool",
				Arg: &Expression{
					Op: "@eq",
					Arg: &Expression{
						Op: "@list",
						Literal: []Expression{
							{Op: "@int", Literal: int64(10)},
							{Op: "@int", Literal: int64(10)},
						},
					},
				},
			}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Bool))
			Expect(reflect.ValueOf(res).Bool()).To(BeTrue())
		})

		It("should deserialize and evaluate a compound bool expression", func() {
			jsonData := `{"@bool":{"@or":[false, true, true, false]}}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Bool))
			Expect(reflect.ValueOf(res).Bool()).To(BeTrue())
		})

		It("should deserialize and evaluate a compound list expression", func() {
			jsonData := `{"@list":[{"@eq":[10,10]},{"@and":[true,false]}]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal([]any{true, false}))
		})

		It("should deserialize and evaluate a compound map expression", func() {
			jsonData := `{"@dict":{"x":{"a":1,"b":2}}}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(Unstructured{"x": Unstructured{"a": int64(1), "b": int64(2)}}))
		})

		It("should deserialize and evaluate a multi-level compound literal expression", func() {
			jsonData := `{"@and": [{"@eq": [10, 10]}, {"@lt": [1, 2]}]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Bool))
			Expect(reflect.ValueOf(res).Bool()).To(BeTrue())
		})

		It("should deserialize and evaluate a compound JSONPath expression", func() {
			jsonData := `{"@lt": ["$.spec.a", "$.spec.b.c"]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Bool))
			Expect(reflect.ValueOf(res).Bool()).To(BeTrue())
		})

		It("should deserialize and evaluate a compound JSONPath expression", func() {
			jsonData := `{"@eq": [{"@len": ["$.spec.x"]}, 5]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Bool))
			Expect(reflect.ValueOf(res).Bool()).To(BeTrue())
		})

		It("should deserialize and evaluate a JSONPath list expression", func() {
			jsonData := `{"@eq": [{"@len": ["$.spec.x"]}, 5]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Bool))
			Expect(reflect.ValueOf(res).Bool()).To(BeTrue())
		})

		It("should deserialize and evaluate a simple list expression", func() {
			jsonData := `{"@sum":[1,2,3]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			kind := reflect.ValueOf(res).Kind()
			Expect(kind).To(Equal(reflect.Int64))
			Expect(res).To(Equal(int64(6)))
		})

		It("should deserialize and evaluate an @in expression", func() {
			jsonData := `{"@in":[1,[1,2,3]]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			kind := reflect.ValueOf(res).Kind()
			Expect(kind).To(Equal(reflect.Bool))
			Expect(res).To(BeTrue())
		})

		It("should deserialize and evaluate an @in expression", func() {
			jsonData := `{"@in":[4,[1,2,3]]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			kind := reflect.ValueOf(res).Kind()
			Expect(kind).To(Equal(reflect.Bool))
			Expect(res).To(BeFalse())
		})

		It("should deserialize and evaluate an @in expression", func() {
			jsonData := `{"@in":["a",[1,2,3]]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			kind := reflect.ValueOf(res).Kind()
			Expect(kind).To(Equal(reflect.Bool))
			Expect(res).To(BeFalse())
		})

		It("should deserialize and evaluate an @in expression", func() {
			jsonData := `{"@in":["nginx",["apache","nginx","nginx"]]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			kind := reflect.ValueOf(res).Kind()
			Expect(kind).To(Equal(reflect.Bool))
			Expect(res).To(BeTrue())
		})
	})

	Describe("Evaluating list expressions", func() {
		It("should deserialize and evaluate a stacked list expression", func() {
			jsonData := `{"@list":[1,2,3]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			kind := reflect.ValueOf(res).Kind()
			Expect(kind == reflect.Slice || kind == reflect.Array).To(BeTrue(),
				fmt.Sprintf("kind mismatch: %v != %v", kind, reflect.Int64))
			Expect(res).To(Equal([]any{int64(1), int64(2), int64(3)}))
		})

		It("should deserialize and evaluate a simple list expression", func() {
			jsonData := `{"@len":[1,2,3]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{
				Op: "@len",
				Arg: &Expression{
					Op: "@list",
					Literal: []Expression{
						{Op: "@int", Literal: int64(1)},
						{Op: "@int", Literal: int64(2)},
						{Op: "@int", Literal: int64(3)},
					},
				},
			}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			kind := reflect.ValueOf(res).Kind()
			Expect(kind).To(Equal(reflect.Int64))
			Expect(res).To(Equal(int64(3)))
		})

		It("should deserialize and evaluate a literal list expression", func() {
			jsonData := `{"dummy":[1,2,3]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{
				Op: "@dict",
				Literal: map[string]Expression{
					"dummy": {
						Op: "@list",
						Literal: []Expression{
							{Op: "@int", Literal: int64(1)},
							{Op: "@int", Literal: int64(2)},
							{Op: "@int", Literal: int64(3)},
						},
					},
				},
			}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			kind := reflect.ValueOf(res).Kind()
			Expect(kind).To(Equal(reflect.Map))
			Expect(reflect.ValueOf(res).Interface().(Unstructured)).
				To(Equal(Unstructured{"dummy": []any{int64(1), int64(2), int64(3)}}))
		})

		It("should deserialize and evaluate a literal list expression with multiple keys", func() {
			jsonData := `{"dummy":[1,2,3],"another-dummy":"a"}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			kind := reflect.ValueOf(res).Kind()
			Expect(kind).To(Equal(reflect.Map))
			Expect(reflect.ValueOf(res).Interface().(Unstructured)).
				To(Equal(Unstructured{"dummy": []any{int64(1), int64(2), int64(3)}, "another-dummy": "a"}))
		})

		It("should deserialize and evaluate a mixed list expression", func() {
			jsonData := `{"dummy":[1,2,3],"x":{"@eq":["a","b"]}}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			kind := reflect.ValueOf(res).Kind()
			Expect(kind).To(Equal(reflect.Map))
			Expect(res).To(Equal(Unstructured{"dummy": []any{int64(1), int64(2), int64(3)}, "x": false}))
		})

		It("should deserialize and evaluate a compound list expression", func() {
			jsonData := `{"another-dummy":[{"a":1,"b":2.2},{"x":[1,2,3]}]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			kind := reflect.ValueOf(res).Kind()
			Expect(kind).To(Equal(reflect.Map))
			Expect(reflect.ValueOf(res).Interface().(Unstructured)).
				To(Equal(Unstructured{"another-dummy": []any{
					Unstructured{"a": int64(1), "b": 2.2},
					Unstructured{"x": []any{int64(1), int64(2), int64(3)}},
				}}))
		})
	})

	Describe("Evaluating label selectors", func() {
		It("should evaluate a @selector expression on a literal labelset", func() {
			jsonData := `{"@selector":[{"matchLabels":{"app":"nginx"}},{"app":"nginx"}]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			v, err := AsBool(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeTrue())
		})

		It("should evaluate a @selector expression using the short form", func() {
			jsonData := `{"@selector":[{"app":"nginx"},{"app":"nginx"}]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			v, err := AsBool(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeTrue())
		})

		It("should evaluate a @selector expression on an object's labels", func() {
			jsonData := `{"@selector":[{"app":"nginx"},"$.metadata.labels"]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			obj := object.DeepCopy(obj1)
			obj.SetLabels(map[string]string{"app": "nginx"})
			ctx := EvalCtx{Object: obj.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			v, err := AsBool(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeTrue())

			obj.SetLabels(map[string]string{"app": "apache"})
			ctx = EvalCtx{Object: obj.UnstructuredContent(), Log: logger}
			res, err = exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			v, err = AsBool(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeFalse())
		})

		It("should evaluate a @selector containing a matchLabels expression on an object's labels", func() {
			jsonData := `{"@selector":[{"matchLabels":{"app":"nginx"}},"$.metadata.labels"]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			obj := object.DeepCopy(obj1)
			obj.SetLabels(map[string]string{"app": "nginx"})
			ctx := EvalCtx{Object: obj.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			v, err := AsBool(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeTrue())

			obj.SetLabels(map[string]string{"app": "apache"})
			ctx = EvalCtx{Object: obj.UnstructuredContent(), Log: logger}
			res, err = exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			v, err = AsBool(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeFalse())
		})

		It("should evaluate a @selector expression with a complex match expression", func() {
			jsonData := `{"@selector":[{"matchExpressions":[{"key":"app","operator":"In","values":["nginx","httpd"]}]},"$.metadata.labels"]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			obj := object.DeepCopy(obj1)
			obj.SetLabels(map[string]string{"app": "nginx"})
			ctx := EvalCtx{Object: obj.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			v, err := AsBool(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeTrue())

			obj.SetLabels(map[string]string{"app": "apache"})
			ctx = EvalCtx{Object: obj.UnstructuredContent(), Log: logger}
			res, err = exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			v, err = AsBool(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeFalse())
		})

		It("should evaluate a @selector expression on an object's labels", func() {
			jsonData := `{"@selector":[{"matchLabels":{"app":"nginx"},"matchExpressions":[{"key":"env","operator":"In","values":["production", "staging"]},{"key":"version","operator": "Exists"}]},"$.metadata.labels"]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			obj := object.DeepCopy(obj1)
			obj.SetLabels(map[string]string{"app": "nginx", "env": "production", "version": "v2"})
			ctx := EvalCtx{Object: obj.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())
			v, err := AsBool(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeTrue())

			obj.SetLabels(map[string]string{"app": "apache", "env": "production", "version": "v2"})
			ctx = EvalCtx{Object: obj.UnstructuredContent(), Log: logger}
			res, err = exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())
			v, err = AsBool(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeFalse())

			obj.SetLabels(map[string]string{"app": "nginx"})
			ctx = EvalCtx{Object: obj.UnstructuredContent(), Log: logger}
			res, err = exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())
			v, err = AsBool(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeFalse())

			obj.SetLabels(map[string]string{"app": "nginx", "env": "staging", "version": "v3"})
			ctx = EvalCtx{Object: obj.UnstructuredContent(), Log: logger}
			res, err = exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())
			v, err = AsBool(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeTrue())
		})
	})

	Describe("Evaluating list commands", func() {
		// @filter
		It("should evaluate a @filter expression on a literal list", func() {
			jsonData := `{"@filter":[{"@eq": ["$$",12]}, [12, 23]]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			vs, err := AsList(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(vs).To(Equal([]any{int64(12)}))
		})

		It("should evaluate a @filter expression with a non-standard root ref on a literal list", func() {
			jsonData := `{"@filter":[{"@eq": ["$$.",12]}, [12, 23]]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			vs, err := AsList(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(vs).To(Equal([]any{int64(12)}))
		})

		It("should evaluate a @filter expression with an object and a local context ", func() {
			jsonData := `{"@filter":[{"@eq":["$.metadata.namespace","$$.metadata.namespace"]},[{"metadata":{"namespace":"default"}},{"metadata":{"namespace":"default2"}},12]]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			vs, err := AsList(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(vs).To(Equal([]any{Unstructured{"metadata": Unstructured{"namespace": "default"}}}))
		})

		// @map
		It("should evaluate a @map expression on a literal list", func() {
			jsonData := `{"@map":[{"x": 1}, [12, 23]]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			vs, err := AsList(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(vs).To(Equal([]any{map[string]any{"x": int64(1)}, map[string]any{"x": int64(1)}}))
		})

		It("should evaluate a @map expression over an arithmetic expression on a literal list", func() {
			jsonData := `{"@map":[{"@lte":["$$",17]}, [12, 23]]}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			vs, err := AsList(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(vs).To(Equal([]any{true, false}))
		})

		It("should evaluate a @map expression generating a list", func() {
			jsonData := `
'@filter':
  - '@exists': "$$"
  - "$.spec.x"`
			var exp Expression
			err := yaml.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			vs, err := AsList(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(vs).To(Equal([]any{int64(1), int64(2), int64(3), int64(4), int64(5)}))
		})

		// It("should evaluate stacked @map expressions", func() {
		// 	jsonData := `{"@map":[{"@lte":["$",2]},{"@first":[{"@map":["$.spec.x"]}]}]}`
		// 	var exp Expression
		// 	err := json.Unmarshal([]byte(jsonData), &exp)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	res, err := exp.Evaluate(eng3)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	vs, err := asList(res)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(vs).To(Equal([]any{true, true, false, false, false}))
		// })

		// It("should evaluate compound @map plus @filter expressions", func() {
		// 	jsonData := `{"@filter":[{"@lte":["$",2]},{"@first":[{"@map":["$.spec.x"]}]}]}`
		// 	var exp Expression
		// 	err := json.Unmarshal([]byte(jsonData), &exp)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	res, err := exp.Evaluate(eng3)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	vs, err := asList(res)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(vs).To(Equal([]any{int64(1), int64(2)}))
		// })

		// // @fold
		// It("should evaluate a simple @fold expression", func() {
		// 	jsonData := `{"@fold":[{"@filter":{"@eq":["$.metadata.namespace","default2"]}},` +
		// 		`{"@map":{"$.metadata.name":"$.metadata.namespace"}}]}`
		// 	var exp Expression
		// 	err := json.Unmarshal([]byte(jsonData), &exp)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	res, err := exp.Evaluate(eng3)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	objs, err := asObjectList(eng3.view, res)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(objs).To(HaveLen(1))
		// 	Expect(objs[0]).To(Equal(map[string]any{
		// 		"metadata": map[string]any{"name": "default2"},
		// 	}))
		// })

		// It("should evaluate an expression that folds multiple objects", func() {
		// 	jsonData := `{"@len":{"@map":"$"}}`
		// 	var exp Expression
		// 	err := json.Unmarshal([]byte(jsonData), &exp)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	res, err := exp.Evaluate(eng3)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(res).To(Equal(int64(2)))
		// })

		// It("should evaluate an expression that folds multiple objects into a valid object", func() {
		// 	jsonData := `{"metadata":{"name":"some-name"},"len":{"@len":{"@map":"$"}}}`
		// 	var exp Expression
		// 	err := json.Unmarshal([]byte(jsonData), &exp)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	res, err := exp.Evaluate(eng3)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(res).To(Equal(map[string]any{
		// 		"metadata": map[string]any{"name": "some-name"},
		// 		"len":      int64(2),
		// 	}))
		// })
	})

	Describe("Evaluating literal @dict expressions", func() {
		It("should deserialize and evaluate a constant literal map expression", func() {
			jsonData := `{"a":1, "b":{"c":"x"}}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp).To(Equal(Expression{
				Op: "@dict",
				Literal: map[string]Expression{
					"a": {Op: "@int", Literal: int64(1)},
					"b": {
						Op: "@dict",
						Literal: map[string]Expression{
							"c": {Op: "@string", Literal: "x"},
						},
					},
				},
			}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Map))
			Expect(res).To(Equal(Unstructured{"a": int64(1), "b": Unstructured{"c": "x"}}))
		})

		It("should deserialize and evaluate a compound literal map expression", func() {
			jsonData := `{"a":1.1,"b":{"@sum":[1,2]},"c":{"@concat":["ab","ba"]}}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			Expect(exp.Op).To(Equal("@dict"))
			Expect(reflect.ValueOf(exp.Literal).MapIndex(reflect.ValueOf("a")).Interface().(Expression)).
				To(Equal(Expression{Op: "@float", Literal: 1.1}))
			Expect(reflect.ValueOf(exp.Literal).MapIndex(reflect.ValueOf("b")).Interface().(Expression)).
				To(Equal(Expression{
					Op: "@sum",
					Arg: &Expression{
						Op: "@list",
						Literal: []Expression{
							{Op: "@int", Literal: int64(1)},
							{Op: "@int", Literal: int64(2)},
						},
					},
				}))
			Expect(reflect.ValueOf(exp.Literal).MapIndex(reflect.ValueOf("c")).Interface().(Expression)).
				To(Equal(Expression{
					Op: "@concat",
					Arg: &Expression{
						Op: "@list",
						Literal: []Expression{
							{Op: "@string", Literal: "ab"},
							{Op: "@string", Literal: "ba"},
						},
					},
				}))

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Map))
			Expect(res).To(Equal(Unstructured{"a": 1.1, "b": int64(3), "c": "abba"}))
		})
	})

	Describe("Evaluating cornercases", func() {
		It("should deserialize and evaluate an expression inside a literal map", func() {
			jsonData := `{"a":1,"b":{"c":{"@eq":[1,1]}}}`
			var exp Expression
			err := json.Unmarshal([]byte(jsonData), &exp)
			Expect(err).NotTo(HaveOccurred())

			ctx := EvalCtx{Object: obj1.UnstructuredContent(), Log: logger}
			res, err := exp.Evaluate(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(reflect.ValueOf(res).Kind()).To(Equal(reflect.Map))
			Expect(res).To(Equal(Unstructured{"a": int64(1), "b": Unstructured{"c": true}}))
		})
	})
})
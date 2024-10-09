package pipeline

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	toolscache "k8s.io/client-go/tools/cache"

	"hsnlab/dcontroller/pkg/cache"
	"hsnlab/dcontroller/pkg/expression"
	"hsnlab/dcontroller/pkg/object"
	"hsnlab/dcontroller/pkg/util"
)

type unstruct = map[string]any

var ObjectKey = toolscache.MetaObjectToName

// defaultEngine is the default implementation of the pipeline engine.
type defaultEngine struct {
	targetView    string               // the views/objects to work on
	baseviews     []gvk                // the view to put the output objects into
	baseViewStore map[gvk]*cache.Store // internal view cache
	log           logr.Logger
}

func NewDefaultEngine(targetView string, baseviews []gvk, log logr.Logger) Engine {
	return &defaultEngine{
		targetView:    targetView,
		baseviews:     baseviews,
		baseViewStore: make(map[gvk]*cache.Store),
		log:           log,
	}
}

func (eng *defaultEngine) Log() logr.Logger { return eng.log }
func (eng *defaultEngine) View() string     { return eng.targetView }

func (eng *defaultEngine) WithObjects(objs ...object.Object) {
	for _, o := range objs {
		gvk := o.GetObjectKind().GroupVersionKind()
		eng.initViewStore(gvk)
		eng.baseViewStore[gvk].Add(o) //nolint:errcheck
	}
}

func (eng *defaultEngine) IsValidEvent(delta cache.Delta) bool {
	if delta.Object == nil {
		return false
	}

	gvk := delta.Object.GetObjectKind().GroupVersionKind()
	eng.initViewStore(gvk)

	if delta.Type == cache.Added || delta.Type == cache.Updated ||
		delta.Type == cache.Upserted || delta.Type == cache.Replaced {
		obj, ok, err := eng.baseViewStore[gvk].GetByKey(ObjectKey(delta.Object).String())
		if err == nil && ok {
			// duplicate0>not-valid
			return !object.DeepEqual(delta.Object, obj)
		}
	}

	return true
}

func (eng *defaultEngine) EvaluateAggregation(a *Aggregation, delta cache.Delta) ([]cache.Delta, error) {
	if delta.IsUnchanged() {
		return []cache.Delta{delta}, nil
	}

	gvk := delta.Object.GetObjectKind().GroupVersionKind()
	eng.initViewStore(gvk)

	if !eng.IsValidEvent(delta) {
		eng.log.V(4).Info("aggregation: ignoring duplicate event", "GVK", gvk,
			"event-type", delta.Type)
		return []cache.Delta{}, nil
	}

	// find out whether an upsert is an update/replace or an add
	delta = eng.handleUpsertEvent(delta)

	ds, err := eng.evaluateAggregation(a, delta)
	if err != nil {
		return nil, err
	}

	eng.log.V(4).Info("aggregation: ready", "event-type", delta.Type, "result", util.Stringify(ds))

	return ds, nil
}

func (eng *defaultEngine) evaluateAggregation(a *Aggregation, delta cache.Delta) ([]cache.Delta, error) {
	gvk := delta.Object.GetObjectKind().GroupVersionKind()

	// update local view cache
	var ds []cache.Delta
	switch delta.Type { //nolint:exhaustive
	case cache.Added:
		eng.log.V(6).Info("aggregation: add using new object", "object", delta.Object)

		objs, err := eng.evalAggregation(a, object.DeepCopy(delta.Object))
		if err != nil {
			return nil, NewAggregationError(
				fmt.Errorf("processing event %q: could not evaluate aggregation for new object %s: %w",
					delta.Type, ObjectKey(delta.Object), err))
		}

		if err := eng.baseViewStore[gvk].Add(delta.Object); err != nil {
			return nil, NewAggregationError(
				fmt.Errorf("processing event %q: could not add object %s to store: %w",
					delta.Type, ObjectKey(delta.Object), err))
		}

		ds = []cache.Delta{}
		for _, obj := range objs {
			// @select shortcuts
			ds = append(ds, cache.Delta{Type: cache.Added, Object: obj})
		}

	case cache.Updated, cache.Replaced:
		eng.log.V(6).Info("aggregate: replacing event with a Delete followed by an Add",
			"event-type", delta.Type, "object", delta.Object)

		delDeltas, err := eng.evaluateAggregation(a, cache.Delta{Type: cache.Deleted, Object: delta.Object})
		if err != nil {
			return nil, NewAggregationError(err)
		}
		delDelta := cache.NilDelta
		if len(delDeltas) == 1 {
			delDelta = delDeltas[0]
		}

		addDeltas, err := eng.evaluateAggregation(a, cache.Delta{Type: cache.Added, Object: delta.Object})
		if err != nil {
			return nil, NewAggregationError(err)
		}
		addDelta := cache.NilDelta
		if len(addDeltas) == 1 {
			addDelta = addDeltas[0]
		}

		// consolidate: objects both in the deleted and added cache are updated
		switch {
		case delDelta.IsUnchanged() && addDelta.IsUnchanged():
			// nothing happened: object wasn't in the view and it still isn't
			// ds = []cache.Delta{{Type: cache.Updated, Object: nil}}
			ds = []cache.Delta{}
		case delDelta.IsUnchanged() && !addDelta.IsUnchanged():
			// object added into the view
			ds = []cache.Delta{addDelta}
		case !delDelta.IsUnchanged() && addDelta.IsUnchanged():
			// object removed from the view
			ds = []cache.Delta{delDelta}
		case ObjectKey(delDelta.Object) == ObjectKey(addDelta.Object):
			// object updated
			ds = []cache.Delta{{Type: cache.Updated, Object: addDelta.Object}}
		default:
			// aggregation affects the name and the name has changed!
			ds = []cache.Delta{delDelta, addDelta}
		}

	case cache.Deleted:
		old, ok, err := eng.baseViewStore[gvk].GetByKey(ObjectKey(delta.Object).String())
		if err != nil {
			return nil, NewAggregationError(err)
		}
		if !ok {
			eng.log.V(4).Info("aggregation: ignoring delete event for an unknown object",
				"event-type", delta.Type, "object", ObjectKey(delta.Object))
			return nil, nil
		}

		eng.log.V(6).Info("aggregation: delete using existing object", "object", old)

		objs, err := eng.evalAggregation(a, object.DeepCopy(old))
		if err != nil {
			return nil, NewAggregationError(
				fmt.Errorf("processing event %q: could not evaluate aggregation for deleted object %s: %w",
					delta.Type, ObjectKey(delta.Object), err))
		}

		if err := eng.baseViewStore[gvk].Delete(old); err != nil {
			return nil, NewAggregationError(
				fmt.Errorf("procesing event %q: could not delete object %s from store: %w",
					delta.Type, ObjectKey(delta.Object), err))
		}

		ds = []cache.Delta{}
		for _, obj := range objs {
			// @select shortcuts
			ds = append(ds, cache.Delta{Type: cache.Deleted, Object: obj})
		}

	default:
		eng.log.V(4).Info("aggregate: ignoring event", "event-type", delta.Type)

		return []cache.Delta{}, nil
	}

	return ds, nil
}

func (eng *defaultEngine) evalAggregation(a *Aggregation, obj object.Object) ([]object.Object, error) {
	args := []unstruct{obj.UnstructuredContent()}
	for _, s := range a.Expressions {
		sres := []unstruct{}
		for _, u := range args {
			ret, err := eng.evalStage(&s, u)
			if err != nil {
				return nil, err
			}
			sres = append(sres, ret...)
		}
		args = sres
	}

	ret := []object.Object{}
	for _, u := range args {
		obj, err := Normalize(eng, u)
		if err != nil {
			return nil, err
		}
		ret = append(ret, obj)
	}

	eng.Log().V(5).Info("eval ready", "aggregation", a.String(), "result", ret)

	return ret, nil
}

func (eng *defaultEngine) evalStage(e *expression.Expression, u unstruct) ([]unstruct, error) {
	if e.Arg == nil {
		return nil, NewAggregationError(
			fmt.Errorf("no expression found in aggregation stage %s", e.String()))
	}

	switch e.Op {
	// @select is one-to-one or one-to-zero
	case "@select":
		res, err := e.Arg.Evaluate(expression.EvalCtx{Object: u, Log: eng.log})
		if err != nil {
			return nil, err
		}

		b, err := expression.AsBool(res)
		if err != nil {
			return nil, NewAggregationError(
				fmt.Errorf("expected conditional expression to evaluate to "+
					"boolean: %w", err))
		}

		// default is no change
		var vs []unstruct
		if b {
			vs = []unstruct{u}
		}

		eng.log.V(5).Info("eval ready", "aggregation", e.String(), "result", vs)

		return vs, nil

	// @project is one-to-one
	case "@project":
		res, err := e.Arg.Evaluate(expression.EvalCtx{Object: u, Log: eng.log})
		if err != nil {
			return nil, err
		}

		v, err := expression.AsObject(res)
		if err != nil {
			return nil, NewAggregationError(err)
		}

		eng.log.V(5).Info("eval ready", "aggregation", e.String(), "result", v)

		return []unstruct{v}, nil

	default:
		return nil, NewAggregationError(
			errors.New("unknown aggregation stage"))
	}
}

func (eng *defaultEngine) EvaluateJoin(j *Join, delta cache.Delta) ([]cache.Delta, error) {
	ds, err := eng.evaluateJoin(j, delta)
	if err != nil {
		return ds, err
	}

	return ds, nil
}

func (eng *defaultEngine) evaluateJoin(j *Join, delta cache.Delta) ([]cache.Delta, error) {
	eng.log.V(5).Info("join: processing event", "event-type", delta.Type, "object", ObjectKey(delta.Object))

	gvk := delta.Object.GetObjectKind().GroupVersionKind()
	eng.initViewStore(gvk)

	if !eng.IsValidEvent(delta) {
		eng.log.V(4).Info("aggregation: ignoring duplicate event", "GVK", gvk,
			"event-type", delta.Type)
		return []cache.Delta{}, nil
	}

	// find out whether an upsert is an update/replace or an add
	delta = eng.handleUpsertEvent(delta)

	ds := make([]cache.Delta, 0)
	switch delta.Type { //nolint:exhaustive
	case cache.Added:
		os, err := eng.evalJoin(j, delta.Object)
		if err != nil {
			return nil, NewJoinError(
				fmt.Errorf("processing event %q: could not evaluate join for new object %s: %w",
					delta.Type, ObjectKey(delta.Object), err))
		}

		if err := eng.baseViewStore[gvk].Add(delta.Object); err != nil {
			return nil, NewJoinError(
				fmt.Errorf("processing event %q: could not add object %s to store: %w",
					delta.Type, ObjectKey(delta.Object), err))
		}

		for _, o := range os {
			ds = append(ds, cache.Delta{Type: cache.Added, Object: o})
		}

	case cache.Updated, cache.Replaced:
		eng.log.V(2).Info("join: replacing event with a Delete followed by an Add",
			"event-type", delta.Type, "object", delta.Object)

		delDeltas, err := eng.evaluateJoin(j, cache.Delta{Type: cache.Deleted, Object: delta.Object})
		if err != nil {
			return nil, NewJoinError(err)
		}

		addDeltas, err := eng.evaluateJoin(j, cache.Delta{Type: cache.Added, Object: delta.Object})
		if err != nil {
			return nil, NewJoinError(err)
		}

		// consolidate: objects both in the deleted and added cache are updated
		a, m, d := diffDeltas(delDeltas, addDeltas)
		ds = append(ds, d...)
		ds = append(ds, m...)
		ds = append(ds, a...)

	case cache.Deleted:
		old, ok, err := eng.baseViewStore[gvk].GetByKey(ObjectKey(delta.Object).String())
		if err != nil {
			return nil, NewJoinError(err)
		}
		if !ok {
			eng.log.V(4).Info("join: ignoring delete event for an unknown object",
				"event-type", delta.Type, "object", ObjectKey(delta.Object))
			return []cache.Delta{}, nil
		}

		eng.log.V(4).Info("join: delete using existing object", "object", old)

		os, err := eng.evalJoin(j, old)
		if err != nil {
			return nil, NewJoinError(
				fmt.Errorf("procesing event %q: could not evaluate join for deleted object %s: %w",
					delta.Type, ObjectKey(delta.Object), err))
		}

		if err := eng.baseViewStore[gvk].Delete(old); err != nil {
			return nil, NewJoinError(
				fmt.Errorf("procesing event %q: could not delete object %s from store: %w",
					delta.Type, ObjectKey(delta.Object), err))
		}

		for _, o := range os {
			ds = append(ds, cache.Delta{Type: cache.Deleted, Object: o})
		}

	default:
		eng.log.V(4).Info("join: ignoring event", "event-type", delta.Type)

		return []cache.Delta{}, nil
	}

	eng.log.V(4).Info("join: ready", "event-type", delta.Type, "result", util.Stringify(ds))

	return ds, nil
}

func (eng *defaultEngine) evalJoin(j *Join, obj object.Object) ([]object.Object, error) {
	res, err := eng.product(obj, func(obj object.Object, current []object.Object) (object.Object, bool, error) {
		// temporary view name: Normalize will eventually recast the object into the target view
		newObj := object.NewViewObject("__tmp_join_view")
		input := newObj.UnstructuredContent()
		ids := []string{}
		for _, v := range current {
			if v == nil {
				continue
			}
			// this may break when working on native K8s objects in different groups
			// that have the same kind (don't do join on native objects!)
			kind := v.GetObjectKind().GroupVersionKind().Kind
			input[kind] = v.UnstructuredContent()
			ids = append(ids, fmt.Sprintf("%s:%s:%s", kind, v.GetNamespace(), v.GetName()))
		}

		// set id: this is needed so that we can disambiguate objects in diffDeltas
		slices.Sort(ids)
		input["metadata"] = map[string]any{"name": strings.Join(ids, "--")}

		// evalutate conditional expression on the input
		res, err := j.Expression.Evaluate(expression.EvalCtx{Object: input, Log: eng.log})
		if err != nil {
			return nil, false, expression.NewExpressionError(&j.Expression, err)
		}

		arg, err := expression.AsBool(res)
		if err != nil {
			return nil, false, expression.NewExpressionError(&j.Expression, err)
		}

		if !arg {
			return nil, false, nil
		}

		// just to make sure
		// newObj.SetUnstructuredContent(input)
		object.SetContent(newObj, input)

		// add input to the join list
		return newObj.DeepCopy(), true, nil
	})
	if err != nil {
		return nil, expression.NewExpressionError(&j.Expression, err)
	}

	eng.log.V(5).Info("eval ready", "expression", j.String(), "result", res)

	return res, nil
}

// product takes an object and a condition expression, generates the Cartesian product of the
// object stored in all the baseviews, applies the expression to each combination, and if it
// evalutates to true then it adds the combined object to the result set
type joinEvalFunc = func(object.Object, []object.Object) (object.Object, bool, error)

func (eng *defaultEngine) product(obj object.Object, eval joinEvalFunc) ([]object.Object, error) {
	if len(eng.baseviews) < 2 {
		return nil, errors.New("at least two views required")
	}

	current, ret := []object.Object{}, []object.Object{}
	err := eng.recurseProd(obj, current, &ret, eval, 0) // pass slice ref: append reallocates it!
	if err != nil {
		return nil, err
	}

	return ret, nil
}

func (eng *defaultEngine) recurseProd(obj object.Object, current []object.Object, ret *([]object.Object), eval joinEvalFunc, depth int) error {
	if depth == len(eng.baseviews) {
		newObj, ok, err := eval(obj, current)
		if err != nil {
			return err
		}
		if ok {
			*ret = append(*ret, newObj)
		}
		return nil
	}

	// skip object's view
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk == eng.baseviews[depth] {
		next := make([]object.Object, len(current))
		copy(next, current)
		next = append(next, obj)
		return eng.recurseProd(obj, next, ret, eval, depth+1)
	}

	store, ok := eng.baseViewStore[eng.baseviews[depth]]
	if !ok {
		// no element seen yet: go on with an empty object
		next := make([]object.Object, len(current))
		copy(next, current)
		next = append(next, nil)
		return eng.recurseProd(obj, next, ret, eval, depth+1)
	}

	for _, o := range store.List() {
		next := make([]object.Object, len(current))
		copy(next, current)
		next = append(next, o)
		err := eng.recurseProd(obj, next, ret, eval, depth+1)
		if err != nil {
			return err
		}
	}

	return nil
}

func (eng *defaultEngine) initViewStore(gvk gvk) {
	if _, ok := eng.baseViewStore[gvk]; !ok {
		eng.baseViewStore[gvk] = cache.NewStore()
	}
}

// find out whether an upsert is an add or an update/replace
func (eng *defaultEngine) handleUpsertEvent(delta cache.Delta) cache.Delta {
	if delta.Type != cache.Upserted {
		return delta
	}

	gvk := delta.Object.GetObjectKind().GroupVersionKind()
	if _, exists, err := eng.baseViewStore[gvk].Get(delta.Object); err != nil || !exists {
		return cache.Delta{Type: cache.Added, Object: delta.Object}
	}

	return cache.Delta{Type: cache.Updated, Object: delta.Object}
}

// helpers
func diffDeltas(dels, adds []cache.Delta) ([]cache.Delta, []cache.Delta, []cache.Delta) {
	a, m, d := []cache.Delta{}, []cache.Delta{}, []cache.Delta{}

	for _, delta := range dels {
		if !containsDelta(adds, delta) {
			d = append(d, cache.Delta{Type: cache.Deleted, Object: delta.Object})
		}
	}

	for _, delta := range adds {
		if containsDelta(dels, delta) {
			m = append(m, cache.Delta{Type: cache.Updated, Object: delta.Object})
		} else {
			a = append(a, cache.Delta{Type: cache.Added, Object: delta.Object})
		}
	}

	return a, m, d
}

func containsDelta(ds []cache.Delta, delta cache.Delta) bool {
	return slices.ContainsFunc(ds, func(n cache.Delta) bool {
		if delta.Object == nil || n.Object == nil {
			return false
		}
		return delta.Object.GetObjectKind().GroupVersionKind() ==
			n.Object.GetObjectKind().GroupVersionKind() &&
			delta.Object.GetName() == n.Object.GetName()
	})
}
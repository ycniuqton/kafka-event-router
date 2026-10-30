package enginetest

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
)

// PayloadSize is the approximate encoded size of a benchmark order event.
type PayloadSize int

const (
	Payload1KB   PayloadSize = 1 << 10
	Payload10KB  PayloadSize = 10 << 10
	Payload100KB PayloadSize = 100 << 10
)

// BenchPayloadSizes is the payload axis of the S1 engine matrix.
var BenchPayloadSizes = []PayloadSize{Payload1KB, Payload10KB, Payload100KB}

// Label names the size for benchmark output (1KB, 10KB, 100KB).
func (s PayloadSize) Label() string { return fmt.Sprintf("%dKB", int(s)/int(Payload1KB)) }

// OrderPayload builds an OrderPaid JSON event whose items array grows until the target size.
type OrderPayload struct {
	Size PayloadSize
}

// orderTemplate holds the fixed parts of the order event.
type orderTemplate string

const (
	orderHead orderTemplate = `{"eventType":"OrderPaid","amount":250.5,"currency":"EUR","cust_id":"C-123456","email":"jane.doe@example.com",` +
		`"card":"4111111111111111","orderId":"o-987654321","createdAt":"2025-12-31T12:00:00Z","items":[`
	orderTail orderTemplate = `]}`
	orderItem orderTemplate = `{"sku":"SKU-%05d","qty":%d,"price":%d.99,"name":"Item number %d with a name"}`
)

// Bytes renders the event; the result is at least one item and within one item of Size.
func (p OrderPayload) Bytes() []byte {
	var sb strings.Builder
	sb.WriteString(string(orderHead))
	for i := 0; i == 0 || sb.Len()+len(orderTail) < int(p.Size); i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, string(orderItem), i, i%9+1, 10+i%90, i)
	}
	sb.WriteString(string(orderTail))
	return []byte(sb.String())
}

// FilterTree is an AND of Conditions conditions that all match OrderPayload, so every one is evaluated.
type FilterTree struct {
	Conditions int
}

// filterTemplate holds the condition shapes the tree cycles through.
type filterTemplate string

const (
	condEventType filterTemplate = `{"type":"cond","field":"value.eventType","op":"eq","value":"OrderPaid"}`
	condAmount    filterTemplate = `{"type":"cond","field":"value.amount","op":"gt","value":%d}`
	condCurrency  filterTemplate = `{"type":"cond","field":"value.currency","op":"in","values":["EUR","USD"]}`
	filterAnd     filterTemplate = `{"type":"and","children":[%s]}`
)

// JSON renders the filter document.
func (f FilterTree) JSON() string {
	conds := make([]string, 0, f.Conditions)
	shapes := []filterTemplate{condEventType, condAmount, condCurrency}
	for i := 0; i < f.Conditions; i++ {
		cond := string(shapes[i%len(shapes)])
		if strings.Contains(cond, "%d") {
			cond = fmt.Sprintf(cond, i%100)
		}
		conds = append(conds, cond)
	}
	return fmt.Sprintf(string(filterAnd), strings.Join(conds, ","))
}

// StepChain is a chain of Steps cheap recipe steps (set, mask, default_if_missing) on distinct fields.
type StepChain struct {
	Steps int
}

// stepTemplate holds the recipe step shapes the chain cycles through.
type stepTemplate string

const (
	stepSet     stepTemplate = `{"kind":"set","field":"value.tag%d","source":{"from":"constant","value":"router"}}`
	stepMask    stepTemplate = `{"kind":"mask","field":"value.email","style":"email"}`
	stepDefault stepTemplate = `{"kind":"default_if_missing","field":"value.opt%d","value":"eu"}`
)

// Items renders each step document.
func (c StepChain) Items() []string {
	steps := make([]string, 0, c.Steps)
	shapes := []stepTemplate{stepSet, stepMask, stepDefault}
	for i := 0; i < c.Steps; i++ {
		step := string(shapes[i%len(shapes)])
		if strings.Contains(step, "%d") {
			step = fmt.Sprintf(step, i)
		}
		steps = append(steps, step)
	}
	return steps
}

// JSON renders the steps array.
func (c StepChain) JSON() string { return StepList(c.Items()).JSON() }

// StepList is an ordered list of step documents.
type StepList []string

// JSON renders the list as a steps array.
func (l StepList) JSON() string { return "[" + strings.Join(l, ",") + "]" }

// Bench route fragments shared by engine, runner and sandbox benchmarks.
const (
	BenchFilterNone = "null"
	BenchNoSteps    = "[]"
	// BenchRecipe5 is a five-step recipe chain: rename, two masks, set, remove.
	BenchRecipe5 = `[{"kind":"rename","from":"value.cust_id","to":"value.customerId"},{"kind":"mask","field":"value.email","style":"email"},
{"kind":"mask","field":"value.card","style":"last4"},{"kind":"set","field":"value.source","source":{"from":"constant","value":"router"}},
{"kind":"remove","fields":["value.createdAt"]}]`
	// BenchMapper reshapes the order with field, constant, formula and metadata leaves.
	BenchMapper = `[{"kind":"map","start":"empty","target":{"fields":{
 "order":{"fields":{"id":{"source":{"from":"field","path":"value.orderId"}},"total":{"source":{"from":"formula","expr":"round(value.amount * 1.19, 2)"}},
  "currency":{"source":{"from":"field","path":"value.currency"}},"lines":{"source":{"from":"field","path":"value.items"}}}},
 "customer":{"fields":{"id":{"source":{"from":"field","path":"value.cust_id"}},"email":{"source":{"from":"formula","expr":"mask(value.email, \"email\")"}}}},
 "source":{"source":{"from":"constant","value":"router"}},"topic":{"source":{"from":"metadata","name":"topic"}}}}}]`
	// BenchTransformJS does the BenchRecipe5 work in JavaScript.
	BenchTransformJS = `function transform(event, ctx) {
  const v = event.value;
  v.customerId = v.cust_id;
  delete v.cust_id;
  v.email = ctx.mask.email(v.email);
  v.card = ctx.mask.last4(v.card);
  v.source = "router";
  delete v.createdAt;
  return event;
}`
)

// BenchCodecEnv selects the codec of the S1 benchmarks (same key and values as the worker).
const BenchCodecEnv = "ROUTER_ENGINE_CODEC"

// BenchCodec returns the codec mode benchmarks compile with (default: the engine default).
func BenchCodec(tb testing.TB) engine.CodecMode {
	m, err := engine.CodecModes{}.Parse(os.Getenv(BenchCodecEnv))
	if err != nil {
		tb.Fatal(err)
	}
	return m
}

// BenchJSStep wraps BenchTransformJS as a js step document.
var BenchJSStep = fmt.Sprintf(`{"id":"js","kind":"js","code":%q}`, BenchTransformJS)

// ShapeName identifies one step-type row of the S1 matrix.
type ShapeName string

const (
	ShapeFilterOnly ShapeName = "filter-only"
	ShapeRecipe5    ShapeName = "recipe-5"
	ShapeMapper     ShapeName = "mapper"
	ShapeJS         ShapeName = "js"
	ShapeFilter1    ShapeName = "filter-1"
	ShapeFilter10   ShapeName = "filter-10"
	ShapeFilter50   ShapeName = "filter-50"
	ShapeSteps1     ShapeName = "steps-1"
	ShapeSteps5     ShapeName = "steps-5"
	ShapeSteps20    ShapeName = "steps-20"
	ShapeSteps5JS   ShapeName = "steps-5-js"
)

// BenchDefault holds the fixed knobs of the matrix: the condition count of the default bench filter
// and the route jsTimeoutMs of JS shapes (the sandbox maximum, so large payloads measure cost
// instead of failing on the 50 ms default).
type BenchDefault int

const (
	standardConditions BenchDefault = 3
	BenchJSTimeoutMs   BenchDefault = 1000
)

// BenchShape is one route shape of the matrix; NeedsJS shapes must compile with a real sandbox runtime.
type BenchShape struct {
	Name    ShapeName
	Filter  string
	Steps   string
	NeedsJS bool
}

// Route renders the single-branch route document of the shape.
func (s BenchShape) Route() string {
	route := RouteJSON(s.Filter, s.Steps)
	if !s.NeedsJS {
		return route
	}
	return strings.TrimSuffix(route, "}") + fmt.Sprintf(`,"limits":{"jsTimeoutMs":%d}}`, BenchJSTimeoutMs)
}

// BenchShapes is the step-type axis of the S1 engine matrix.
var BenchShapes = []BenchShape{
	{Name: ShapeFilterOnly, Filter: FilterTree{Conditions: int(standardConditions)}.JSON(), Steps: BenchNoSteps},
	{Name: ShapeRecipe5, Filter: FilterTree{Conditions: int(standardConditions)}.JSON(), Steps: BenchRecipe5},
	{Name: ShapeMapper, Filter: FilterTree{Conditions: int(standardConditions)}.JSON(), Steps: BenchMapper},
	{Name: ShapeJS, Filter: FilterTree{Conditions: int(standardConditions)}.JSON(), Steps: StepList{BenchJSStep}.JSON(), NeedsJS: true},
	{Name: ShapeFilter1, Filter: FilterTree{Conditions: 1}.JSON(), Steps: BenchNoSteps},
	{Name: ShapeFilter10, Filter: FilterTree{Conditions: 10}.JSON(), Steps: BenchNoSteps},
	{Name: ShapeFilter50, Filter: FilterTree{Conditions: 50}.JSON(), Steps: BenchNoSteps},
	{Name: ShapeSteps1, Filter: BenchFilterNone, Steps: StepChain{Steps: 1}.JSON()},
	{Name: ShapeSteps5, Filter: BenchFilterNone, Steps: StepChain{Steps: 5}.JSON()},
	{Name: ShapeSteps20, Filter: BenchFilterNone, Steps: StepChain{Steps: 20}.JSON()},
	{Name: ShapeSteps5JS, Filter: BenchFilterNone, Steps: StepList(append(StepChain{Steps: 5}.Items(), BenchJSStep)).JSON(), NeedsJS: true},
}

// BenchShapeSet selects shapes of the matrix by name.
type BenchShapeSet []BenchShape

// Only keeps the named shapes, in catalog order.
func (s BenchShapeSet) Only(names ...ShapeName) BenchShapeSet {
	out := BenchShapeSet{}
	for _, shape := range s {
		for _, n := range names {
			if shape.Name == n {
				out = append(out, shape)
			}
		}
	}
	return out
}

// Canonical renders the event as the tree codec writes it (sorted keys, compact).
func (p OrderPayload) Canonical() []byte {
	v, err := codec.TreeReader{}.Read(p.Bytes())
	if err != nil {
		panic(err)
	}
	out, err := codec.TreeWriter{}.Write(v)
	if err != nil {
		panic(err)
	}
	return out
}

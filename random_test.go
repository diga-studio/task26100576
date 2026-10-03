package formflow

import (
	"math/rand"
	"reflect"
	"testing"
)

// referenceVisible 是测试内的独立参考实现：
// 以 DFS 搜索每个字段的有限证明（条件被引用字段必须自身可见），
// 访问栈上的回边视为不成立，从而排除“环自行显示”。
// 只缓存证明成功的结果，避免缓存到依赖回边剪枝的假阴性。
func referenceVisible(c *Compiled, answers map[string]string) map[string]bool {
	const (
		unknown = iota
		visiting
		proven
	)
	state := make([]int, len(c.fields))
	var prove func(i int) bool
	prove = func(i int) bool {
		switch state[i] {
		case proven:
			return true
		case visiting:
			return false
		}
		f := c.fields[i]
		if len(f.Groups) == 0 {
			state[i] = proven
			return true
		}
		state[i] = visiting
		for _, group := range f.Groups {
			ok := true
			for _, cond := range group {
				j := c.index[cond.Field]
				answer, answered := answers[cond.Field]
				if !answered || answer != cond.Equals || !prove(j) {
					ok = false
					break
				}
			}
			if ok {
				state[i] = proven
				return true
			}
		}
		state[i] = unknown // 不缓存失败，避免回边剪枝污染
		return false
	}
	visible := make(map[string]bool, len(c.fields))
	for i, f := range c.fields {
		visible[f.Name] = prove(i)
	}
	return visible
}

func referenceAnswers(c *Compiled, answers map[string]string) map[string]string {
	visible := referenceVisible(c, answers)
	view := make(map[string]string)
	for name, value := range answers {
		if visible[name] {
			view[name] = value
		}
	}
	return view
}

// 含环、自引用、前向引用、AND 组与多组 OR 的小型定义。
func randomDef(t *testing.T) *Compiled {
	t.Helper()
	c, err := Compile(Definition{Fields: []FieldDef{
		{Name: "e1"},
		{Name: "e2"},
		{Name: "A", Groups: [][]Condition{
			{{Field: "e1", Equals: "1"}},
			{{Field: "B", Equals: "1"}},
		}},
		{Name: "B", Groups: [][]Condition{
			{{Field: "A", Equals: "1"}},
			{{Field: "C", Equals: "2"}},
		}},
		{Name: "C", Groups: [][]Condition{
			{{Field: "B", Equals: "1"}, {Field: "e2", Equals: "2"}},
			{{Field: "D", Equals: "x"}},
		}},
		{Name: "D", Groups: [][]Condition{
			{{Field: "C", Equals: "2"}},
		}},
		{Name: "E", Groups: [][]Condition{
			{{Field: "E", Equals: "1"}},
		}},
		{Name: "F", Groups: [][]Condition{
			{{Field: "A", Equals: "1"}, {Field: "B", Equals: "1"}},
			{{Field: "e2", Equals: "1"}},
		}},
	}})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}

func randomOps(rng *rand.Rand, fields []string, values []string, n int) []Op {
	perm := rng.Perm(len(fields))
	if n > len(fields) {
		n = len(fields)
	}
	ops := make([]Op, n)
	for i := range ops {
		field := fields[perm[i]]
		if rng.Intn(3) == 0 {
			ops[i] = Clear(field)
		} else {
			ops[i] = Set(field, values[rng.Intn(len(values))])
		}
	}
	return ops
}

// applyDiff 把差量应用到旧的有效答案视图，得到新的视图。
func applyDiff(view map[string]string, diff Diff) map[string]string {
	next := make(map[string]string, len(view))
	for name, value := range view {
		next[name] = value
	}
	for name, state := range diff {
		if state.Visible && state.Answered {
			next[name] = state.Answer
		} else {
			delete(next, name)
		}
	}
	return next
}

func TestRandomBatchesAgainstReference(t *testing.T) {
	c := randomDef(t)
	fields := []string{"e1", "e2", "A", "B", "C", "D", "E", "F"}
	values := []string{"", "1", "2", "x"}
	rng := rand.New(rand.NewSource(20261005))

	s := Open(c, nil)
	sReversed := Open(c, nil)
	prevView := map[string]string{}

	for batch := 0; batch < 300; batch++ {
		ops := randomOps(rng, fields, values, 1+rng.Intn(6))

		diff := s.Patch(ops...)

		// 交换批内顺序：结果必须与处理顺序无关。
		reversed := make([]Op, len(ops))
		for i, op := range ops {
			reversed[len(ops)-1-i] = op
		}
		sReversed.Patch(reversed...)

		wantView := referenceAnswers(c, s.RawAnswers())
		if got := s.Answers(); !reflect.DeepEqual(got, wantView) {
			t.Fatalf("batch %d: Answers() = %v, 参考实现 = %v, raw = %v", batch, got, wantView, s.RawAnswers())
		}
		for name := range c.index {
			if got, want := s.IsVisible(name), referenceVisible(c, s.RawAnswers())[name]; got != want {
				t.Fatalf("batch %d: IsVisible(%q) = %v, 参考实现 = %v", batch, name, got, want)
			}
		}

		// 差量应用到旧视图后必须得到完整最终视图。
		if got := applyDiff(prevView, diff); !reflect.DeepEqual(got, wantView) {
			t.Fatalf("batch %d: 差量应用后视图 = %v, 期望 %v, diff = %v", batch, got, wantView, diff)
		}
		prevView = wantView

		// 批内顺序交换后的会话应达到相同状态。
		if got := sReversed.Answers(); !reflect.DeepEqual(got, wantView) {
			t.Fatalf("batch %d: 逆序批处理后 Answers() = %v, 期望 %v", batch, got, wantView)
		}
		if got, want := sReversed.RawAnswers(), s.RawAnswers(); !reflect.DeepEqual(got, want) {
			t.Fatalf("batch %d: 逆序批处理后 RawAnswers() = %v, 期望 %v", batch, got, want)
		}
	}
}

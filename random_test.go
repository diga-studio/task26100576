package formflow

import (
	"math/rand"
	"reflect"
	"testing"
)

// referenceVisible 是测试内的独立参考实现：
// 用带备忘的递归证明搜索判定每个字段是否存在
// 从无条件字段出发的有限证明（与库内的迭代不动点是不同的算法）。
func referenceVisible(def Definition, answers map[string]string) map[string]bool {
	byName := make(map[string]FieldDef, len(def.Fields))
	for _, f := range def.Fields {
		byName[f.Name] = f
	}
	memo := make(map[string]bool, len(def.Fields))
	visiting := make(map[string]bool, len(def.Fields))
	var provable func(name string) bool
	provable = func(name string) bool {
		if v, ok := memo[name]; ok {
			return v
		}
		f := byName[name]
		if len(f.Groups) == 0 {
			memo[name] = true
			return true
		}
		if visiting[name] {
			return false // 环上不存在沿此路径的有限证明
		}
		visiting[name] = true
		res := false
		for _, g := range f.Groups {
			ok := true
			for _, cond := range g {
				v, filled := answers[cond.Field]
				if !filled || v != cond.Equals || !provable(cond.Field) {
					ok = false
					break
				}
			}
			if ok {
				res = true
				break
			}
		}
		delete(visiting, name)
		memo[name] = res
		return res
	}
	out := make(map[string]bool, len(def.Fields))
	for _, f := range def.Fields {
		out[f.Name] = provable(f.Name)
	}
	return out
}

func referenceEffective(def Definition, answers map[string]string) map[string]string {
	vis := referenceVisible(def, answers)
	out := map[string]string{}
	for name, v := range answers {
		if vis[name] {
			out[name] = v
		}
	}
	return out
}

// randomDef 返回一份含环、自引用、前向引用与多条件组的小型定义。
func randomDef() Definition {
	return Definition{Fields: []FieldDef{
		{Name: "gate1"},
		{Name: "gate2"},
		// 环 a -> b -> c -> a，a、b 各有一条独立外部支撑。
		{Name: "a", Groups: []ConditionGroup{
			{{Field: "b", Equals: "x"}},
			{{Field: "gate1", Equals: "on"}},
		}},
		{Name: "b", Groups: []ConditionGroup{
			{{Field: "c", Equals: "x"}},
			{{Field: "gate2", Equals: "on"}},
		}},
		{Name: "c", Groups: []ConditionGroup{
			{{Field: "a", Equals: "x"}},
		}},
		// 自引用 + 前向引用（d 引用后面才定义的 e）。
		{Name: "d", Groups: []ConditionGroup{
			{{Field: "d", Equals: "y"}},
			{{Field: "e", Equals: "y"}},
		}},
		{Name: "e", Groups: []ConditionGroup{
			{{Field: "gate1", Equals: "y"}, {Field: "gate2", Equals: "y"}},
		}},
		// 无条件字段，作为普通答案载体。
		{Name: "plain"},
	}}
}

func TestRandomBatchesAgainstReference(t *testing.T) {
	def := randomDef()
	c := mustCompile(t, def)
	names := make([]string, 0, len(def.Fields))
	for _, f := range def.Fields {
		names = append(names, f.Name)
	}
	alphabet := []string{"x", "y", "on", "off", ""}

	for _, seed := range []int64{1, 7, 42, 2026, 99991} {
		rng := rand.New(rand.NewSource(seed))
		s := Open(c, nil)
		sRev := Open(c, nil) // 以相反顺序应用每一批，验证批内顺序无关
		// 用差量增量维护的视图（从 Open 的初始状态出发），
		// 验证差量应用后得到完整最终视图。
		viewEffective := map[string]string{}
		viewVisible := map[string]bool{}
		for _, name := range names {
			viewVisible[name] = s.Visible(name)
		}

		for round := 0; round < 200; round++ {
			n := 1 + rng.Intn(4)
			batch := make([]Change, n)
			perm := rng.Perm(len(names))
			for i := range batch {
				batch[i] = Change{Field: names[perm[i]]}
				if rng.Intn(3) == 0 {
					batch[i].Set = false
				} else {
					batch[i].Set = true
					batch[i].Value = alphabet[rng.Intn(len(alphabet))]
				}
			}
			deltas := s.Patch(batch)
			rev := make([]Change, n)
			for i, ch := range batch {
				rev[n-1-i] = ch
			}
			sRev.Patch(rev)

			raw := s.RawAnswers()
			wantVis := referenceVisible(def, raw)
			for _, name := range names {
				if got := s.Visible(name); got != wantVis[name] {
					t.Fatalf("seed=%d round=%d 字段 %s 可见性=%v, 参考=%v (raw=%v)",
						seed, round, name, got, wantVis[name], raw)
				}
			}
			wantEff := referenceEffective(def, raw)
			if got := s.EffectiveAnswers(); !reflect.DeepEqual(got, wantEff) {
				t.Fatalf("seed=%d round=%d 有效答案=%v, 参考=%v (raw=%v)",
					seed, round, got, wantEff, raw)
			}
			// 批内顺序无关。
			if got := sRev.EffectiveAnswers(); !reflect.DeepEqual(got, wantEff) {
				t.Fatalf("seed=%d round=%d 逆序批有效答案=%v, 参考=%v",
					seed, round, got, wantEff)
			}
			for _, name := range names {
				if got := sRev.Visible(name); got != wantVis[name] {
					t.Fatalf("seed=%d round=%d 逆序批字段 %s 可见性=%v, 参考=%v",
						seed, round, name, got, wantVis[name])
				}
			}
			// 差量应用后必须得到完整最终视图。
			for _, d := range deltas {
				viewVisible[d.Field] = d.Visible
				if d.Visible && d.Filled {
					viewEffective[d.Field] = d.Value
				} else {
					delete(viewEffective, d.Field)
				}
			}
			if !reflect.DeepEqual(viewEffective, wantEff) {
				t.Fatalf("seed=%d round=%d 差量应用后视图=%v, 参考=%v",
					seed, round, viewEffective, wantEff)
			}
			for _, name := range names {
				if got := viewVisible[name]; got != wantVis[name] {
					t.Fatalf("seed=%d round=%d 差量应用后字段 %s 可见性=%v, 参考=%v",
						seed, round, name, got, wantVis[name])
				}
			}
			// 差量不得包含未变化的字段。
			for _, d := range deltas {
				if d.Visible && d.Filled && viewEffective[d.Field] != d.Value {
					t.Fatalf("seed=%d round=%d 差量与视图不一致: %+v", seed, round, d)
				}
			}
		}
	}
}

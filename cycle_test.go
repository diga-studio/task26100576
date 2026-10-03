package formflow

import (
	"reflect"
	"testing"
)

// cycleDef 构建测试定义：
// entry、entry2 无条件可见；
// A 在 entry=="是" 或 entry2=="是" 或 B=="是" 时显示；
// B 在 A=="是" 时显示。A、B 构成环，entry/entry2 是两条独立外部支撑。
func cycleDef() Definition {
	return Definition{Fields: []FieldDef{
		{Name: "entry"},
		{Name: "entry2"},
		{Name: "A", Groups: []ConditionGroup{
			{{Field: "entry", Equals: "是"}},
			{{Field: "entry2", Equals: "是"}},
			{{Field: "B", Equals: "是"}},
		}},
		{Name: "B", Groups: []ConditionGroup{
			{{Field: "A", Equals: "是"}},
		}},
	}}
}

func mustCompile(t *testing.T, def Definition) *Compiled {
	t.Helper()
	c, err := Compile(def)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}

func deltaMap(deltas []FieldDelta) map[string]FieldDelta {
	m := make(map[string]FieldDelta, len(deltas))
	for _, d := range deltas {
		m[d.Field] = d
	}
	return m
}

// 外部支撑使循环字段显示，撤销支撑后整环隐藏，恢复支撑后旧答案重新生效。
func TestCycleSupportRevokeRestore(t *testing.T) {
	s := Open(mustCompile(t, cycleDef()), map[string]string{
		"entry": "是", "A": "是", "B": "是",
	})
	if !s.Visible("A") || !s.Visible("B") {
		t.Fatalf("A、B 应在 entry 支撑下可见: A=%v B=%v", s.Visible("A"), s.Visible("B"))
	}
	want := map[string]string{"entry": "是", "A": "是", "B": "是"}
	if got := s.EffectiveAnswers(); !reflect.DeepEqual(got, want) {
		t.Fatalf("有效答案 = %v, 期望 %v", got, want)
	}

	// 撤销支撑：整环必须隐藏，即使 A、B 仍保存着“是”。
	deltas := deltaMap(s.Patch([]Change{{Field: "entry", Set: true, Value: "否"}}))
	if s.Visible("A") || s.Visible("B") {
		t.Fatalf("支撑撤销后 A、B 必须隐藏: A=%v B=%v", s.Visible("A"), s.Visible("B"))
	}
	for _, name := range []string{"A", "B"} {
		d, ok := deltas[name]
		if !ok || d.Visible || d.Filled {
			t.Fatalf("差量应报告 %s 变为隐藏, 实际 %+v (存在=%v)", name, d, ok)
		}
	}
	if _, ok := s.EffectiveAnswers()["A"]; ok {
		t.Fatal("隐藏字段不得出现在有效答案中")
	}
	// 隐藏不等于删除：原始答案保留。
	if got := s.RawAnswers()["A"]; got != "是" {
		t.Fatalf("隐藏字段原始答案应保留, 得到 %q", got)
	}

	// 隐藏期间直接修改隐藏字段：不应产生差量。
	if got := s.Patch([]Change{{Field: "B", Set: true, Value: "暂存"}}); len(got) != 0 {
		t.Fatalf("仅原始隐藏答案变化不应产生差量, 得到 %+v", got)
	}
	if got := s.RawAnswers()["B"]; got != "暂存" {
		t.Fatalf("调用方应能修改未显示字段, 得到 %q", got)
	}

	// 恢复支撑：旧答案（含隐藏期间修改的值）自动恢复生效。
	deltas = deltaMap(s.Patch([]Change{{Field: "entry", Set: true, Value: "是"}}))
	if !s.Visible("A") || !s.Visible("B") {
		t.Fatal("支撑恢复后 A、B 应重新可见")
	}
	want = map[string]string{"entry": "是", "A": "是", "B": "暂存"}
	if got := s.EffectiveAnswers(); !reflect.DeepEqual(got, want) {
		t.Fatalf("恢复后有效答案 = %v, 期望 %v", got, want)
	}
	if d := deltas["B"]; !d.Visible || !d.Filled || d.Value != "暂存" {
		t.Fatalf("差量应报告 B 恢复可见且值为暂存, 实际 %+v", d)
	}
}

// 环有第二条独立支撑时，撤销其中一条不能过度撤回。
func TestCycleSecondIndependentSupport(t *testing.T) {
	s := Open(mustCompile(t, cycleDef()), map[string]string{
		"entry": "是", "entry2": "是", "A": "是", "B": "是",
	})
	deltas := deltaMap(s.Patch([]Change{{Field: "entry", Set: true, Value: "否"}}))
	if _, ok := deltas["A"]; ok {
		t.Fatal("entry2 仍支撑整环, A 不应出现差量")
	}
	if _, ok := deltas["B"]; ok {
		t.Fatal("entry2 仍支撑整环, B 不应出现差量")
	}
	if !s.Visible("A") || !s.Visible("B") {
		t.Fatal("entry2 支撑下 A、B 应保持可见")
	}

	// 两条支撑都撤销后整环才隐藏。
	s.Patch([]Change{{Field: "entry2", Set: false}})
	if s.Visible("A") || s.Visible("B") {
		t.Fatal("两条支撑都撤销后 A、B 必须隐藏")
	}
}

// 无外部支撑时，环内字段即使答案互相满足也不能自行显示。
func TestCycleNeverSelfStarts(t *testing.T) {
	s := Open(mustCompile(t, cycleDef()), map[string]string{"A": "是", "B": "是"})
	if s.Visible("A") || s.Visible("B") {
		t.Fatal("环不能仅靠内部互相满足而自行显示")
	}
	if got := s.EffectiveAnswers(); len(got) != 0 {
		t.Fatalf("有效答案应为空, 得到 %v", got)
	}
}

// 未填写与空字符串不同：Equals "" 只匹配显式填写的空串。
func TestEmptyStringVsUnset(t *testing.T) {
	c := mustCompile(t, Definition{Fields: []FieldDef{
		{Name: "src"},
		{Name: "dep", Groups: []ConditionGroup{{{Field: "src", Equals: ""}}}},
	}})
	s := Open(c, nil)
	if s.Visible("dep") {
		t.Fatal("src 未填写时不应匹配空串条件")
	}
	s.Patch([]Change{{Field: "src", Set: true, Value: ""}})
	if !s.Visible("dep") {
		t.Fatal("src 显式填写空串后应匹配空串条件")
	}
	s.Patch([]Change{{Field: "src", Set: false}})
	if s.Visible("dep") {
		t.Fatal("src 清除后应回到不匹配状态")
	}
}

// 自引用字段：只有外部支撑能让它显示，显示后靠自身答案维持。
func TestSelfReference(t *testing.T) {
	c := mustCompile(t, Definition{Fields: []FieldDef{
		{Name: "gate"},
		{Name: "self", Groups: []ConditionGroup{
			{{Field: "gate", Equals: "on"}},
			{{Field: "self", Equals: "x"}},
		}},
	}})
	s := Open(c, map[string]string{"self": "x"})
	if s.Visible("self") {
		t.Fatal("自引用不能自我启动")
	}
	s.Patch([]Change{{Field: "gate", Set: true, Value: "on"}})
	if !s.Visible("self") {
		t.Fatal("外部支撑应使自引用字段显示")
	}
	s.Patch([]Change{{Field: "gate", Set: false}})
	if s.Visible("self") {
		t.Fatal("支撑撤销后自引用字段必须隐藏")
	}
}

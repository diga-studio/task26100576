package formflow

import (
	"reflect"
	"testing"
)

// 入口 entry 无条件；A 在 entry=="是" 或 B=="是" 时显示；B 在 A=="是" 时显示。
func cycleDef(t *testing.T) *Compiled {
	t.Helper()
	c, err := Compile(Definition{Fields: []FieldDef{
		{Name: "entry"},
		{Name: "A", Groups: [][]Condition{
			{{Field: "entry", Equals: "是"}},
			{{Field: "B", Equals: "是"}},
		}},
		{Name: "B", Groups: [][]Condition{
			{{Field: "A", Equals: "是"}},
		}},
	}})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}

func requireVisibility(t *testing.T, s *Session, want map[string]bool) {
	t.Helper()
	for name, visible := range want {
		if got := s.IsVisible(name); got != visible {
			t.Errorf("IsVisible(%q) = %v, want %v", name, got, visible)
		}
	}
}

func requireAnswers(t *testing.T, s *Session, want map[string]string) {
	t.Helper()
	if got := s.Answers(); !reflect.DeepEqual(got, want) {
		t.Errorf("Answers() = %v, want %v", got, want)
	}
}

// 外部支撑使循环字段显示，撤销支撑后整环隐藏，恢复支撑后旧答案重新生效。
func TestCycleSupportWithdrawRestore(t *testing.T) {
	s := Open(cycleDef(t), map[string]string{
		"entry": "否",
		"A":     "是",
		"B":     "是",
	})

	// 入口不满足时，A、B 仅靠环内互相支撑，不能自行显示。
	requireVisibility(t, s, map[string]bool{"entry": true, "A": false, "B": false})
	requireAnswers(t, s, map[string]string{"entry": "否"})

	// 外部支撑使整环显示，保存的旧答案进入有效视图。
	diff := s.Patch(Set("entry", "是"))
	requireVisibility(t, s, map[string]bool{"A": true, "B": true})
	requireAnswers(t, s, map[string]string{"entry": "是", "A": "是", "B": "是"})
	if !diff["A"].Visible || !diff["B"].Visible {
		t.Errorf("支撑生效后 diff 应包含 A、B 变为可见，got %v", diff)
	}

	// 撤销支撑后整环隐藏，不能沿用旧的可见状态。
	diff = s.Patch(Set("entry", "否"))
	requireVisibility(t, s, map[string]bool{"A": false, "B": false})
	requireAnswers(t, s, map[string]string{"entry": "否"})
	if diff["A"].Visible || diff["B"].Visible {
		t.Errorf("支撑撤销后 diff 应包含 A、B 变为隐藏，got %v", diff)
	}

	// 隐藏不删除：原始答案仍保留在会话中。
	if got := s.RawAnswers(); !reflect.DeepEqual(got, map[string]string{"entry": "否", "A": "是", "B": "是"}) {
		t.Errorf("RawAnswers() = %v, 原始答案应保留", got)
	}

	// 恢复支撑后旧答案自动重新生效。
	s.Patch(Set("entry", "是"))
	requireVisibility(t, s, map[string]bool{"A": true, "B": true})
	requireAnswers(t, s, map[string]string{"entry": "是", "A": "是", "B": "是"})
}

// 环有第二条独立支撑时，撤销其中一条不能过度撤回。
func TestCycleWithSecondIndependentSupport(t *testing.T) {
	c, err := Compile(Definition{Fields: []FieldDef{
		{Name: "e1"},
		{Name: "e2"},
		{Name: "A", Groups: [][]Condition{
			{{Field: "e1", Equals: "是"}},
			{{Field: "B", Equals: "是"}},
		}},
		{Name: "B", Groups: [][]Condition{
			{{Field: "A", Equals: "是"}},
			{{Field: "e2", Equals: "是"}},
		}},
	}})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	s := Open(c, map[string]string{"e1": "是", "e2": "是", "A": "是", "B": "是"})
	requireVisibility(t, s, map[string]bool{"A": true, "B": true})

	// 撤销 e1：B 仍由 e2 支撑，A 仍由 B 支撑，整环保持可见。
	diff := s.Patch(Set("e1", "否"))
	requireVisibility(t, s, map[string]bool{"A": true, "B": true})
	if len(diff) != 1 || diff["e1"].Answer != "否" {
		t.Errorf("撤销 e1 不应影响环的可见性，diff = %v", diff)
	}

	// 再撤销 e2：两条支撑都没了，整环隐藏。
	diff = s.Patch(Set("e2", "否"))
	requireVisibility(t, s, map[string]bool{"A": false, "B": false})
	if diff["A"].Visible || diff["B"].Visible {
		t.Errorf("两条支撑都撤销后整环应隐藏，diff = %v", diff)
	}
}

// 隐藏字段不能支撑其他字段；直接修改隐藏字段不出现在差量中。
func TestHiddenAnswerDoesNotSupport(t *testing.T) {
	c, err := Compile(Definition{Fields: []FieldDef{
		{Name: "gate", Groups: [][]Condition{
			{{Field: "trigger", Equals: "开"}},
		}},
		{Name: "trigger"},
		{Name: "dependent", Groups: [][]Condition{
			{{Field: "hidden", Equals: "x"}},
		}},
		{Name: "hidden", Groups: [][]Condition{
			{{Field: "trigger", Equals: "关"}},
		}},
	}})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	s := Open(c, map[string]string{"trigger": "开", "hidden": "x"})
	requireVisibility(t, s, map[string]bool{"gate": true, "hidden": false, "dependent": false})

	// 仅修改隐藏字段的原始答案：可见性与有效视图不变，diff 为空。
	diff := s.Patch(Set("hidden", "y"))
	if len(diff) != 0 {
		t.Errorf("仅隐藏字段原始答案变化时 diff 应为空，got %v", diff)
	}
	if got := s.RawAnswers()["hidden"]; got != "y" {
		t.Errorf("隐藏字段的原始答案应可修改，got %q", got)
	}
}

// 未填写与空字符串不同；自引用与前向引用。
func TestEmptyVsUnsetAndReferences(t *testing.T) {
	c, err := Compile(Definition{Fields: []FieldDef{
		{Name: "later", Groups: [][]Condition{
			{{Field: "forward", Equals: ""}}, // 前向引用，且要求空字符串答案
		}},
		{Name: "forward"},
		{Name: "self", Groups: [][]Condition{
			{{Field: "self", Equals: "x"}}, // 自引用：无外部支撑时永不显示
		}},
	}})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	// forward 未填写：不等于空字符串，later 隐藏。
	s := Open(c, map[string]string{"self": "x"})
	requireVisibility(t, s, map[string]bool{"forward": true, "later": false, "self": false})

	// forward 填写为空字符串：条件成立，later 显示。
	diff := s.Patch(Set("forward", ""))
	requireVisibility(t, s, map[string]bool{"later": true})
	if !diff["later"].Visible {
		t.Errorf("空字符串答案应满足 Equals 为空的条件，diff = %v", diff)
	}
	requireAnswers(t, s, map[string]string{"forward": ""})

	// 清除后回到未填写，later 重新隐藏。
	s.Patch(Clear("forward"))
	requireVisibility(t, s, map[string]bool{"later": false})
	requireAnswers(t, s, map[string]string{})
}

// 同组条件必须全部满足；多组任意满足。
func TestGroupSemantics(t *testing.T) {
	c, err := Compile(Definition{Fields: []FieldDef{
		{Name: "x"},
		{Name: "y"},
		{Name: "z"},
		{Name: "target", Groups: [][]Condition{
			{{Field: "x", Equals: "1"}, {Field: "y", Equals: "2"}},
			{{Field: "z", Equals: "3"}},
		}},
	}})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	s := Open(c, map[string]string{"x": "1"})
	requireVisibility(t, s, map[string]bool{"target": false})
	s.Patch(Set("y", "2"))
	requireVisibility(t, s, map[string]bool{"target": true})
	s.Patch(Clear("x"), Clear("y"))
	requireVisibility(t, s, map[string]bool{"target": false})
	s.Patch(Set("z", "3"))
	requireVisibility(t, s, map[string]bool{"target": true})
}

// 编译期校验：重复字段与悬空引用。
func TestCompileErrors(t *testing.T) {
	if _, err := Compile(Definition{Fields: []FieldDef{{Name: "a"}, {Name: "a"}}}); err == nil {
		t.Error("重复字段名应报错")
	}
	if _, err := Compile(Definition{Fields: []FieldDef{
		{Name: "a", Groups: [][]Condition{{{Field: "ghost", Equals: "1"}}}},
	}}); err == nil {
		t.Error("引用不存在的字段应报错")
	}
}

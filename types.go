// Package formflow 提供问卷条件字段的显隐求值能力。
//
// 字段没有显隐规则时始终可见；有规则时，规则由若干条件组组成，
// 任意一组满足即可显示，同组内所有条件都必须满足。
// 每个条件表示另一个字段的（原始）答案等于指定字符串，且该字段当前可见。
// 字段可见必须存在从无条件字段出发的有限证明（最小不动点语义）。
package formflow

// Condition 表示一个条件：字段 Field 当前可见，且其答案等于 Equals。
// 被引用字段未填写时条件不成立；空字符串答案与 Equals 为空时成立。
type Condition struct {
	Field  string
	Equals string
}

// FieldDef 描述一个字段。Groups 为空表示该字段无显隐规则、始终可见；
// 否则任意一个条件组内所有条件都满足时字段可见。
type FieldDef struct {
	Name   string
	Groups [][]Condition
}

// Definition 是一份问卷定义，输入保证所有条件引用的字段都存在。
type Definition struct {
	Fields []FieldDef
}

// Op 表示对单个字段的一次赋值或清除。
// Set 为 true 时把字段答案设为 Value（可以是空字符串，与未填写不同）；
// Set 为 false 时清除答案，使字段回到未填写状态。
type Op struct {
	Field string
	Set   bool
	Value string
}

// Set 构造一个赋值操作。
func Set(field, value string) Op { return Op{Field: field, Set: true, Value: value} }

// Clear 构造一个清除操作。
func Clear(field string) Op { return Op{Field: field} }

// FieldState 描述差量中一个字段在整批修改之后的新状态。
type FieldState struct {
	Visible  bool   // 修改后是否可见
	Answered bool   // 修改后有效答案视图中是否包含该字段
	Answer   string // Answered 为 true 时的有效答案
}

// Diff 记录一次 Patch 后可见性或有效答案发生变化的字段及其新状态。
// 仅原始隐藏答案变化的字段不会出现在 Diff 中。
type Diff map[string]FieldState

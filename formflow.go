// Package formflow 提供问卷条件字段的显隐求值能力。
//
// 组件只处理字符串答案，未填写（答案不存在）与空字符串是不同的状态。
// 字段没有显隐规则时始终可见；有规则时，规则由若干条件组组成，
// 任意一组满足即可显示，同组内所有条件都必须满足。
// 每个条件表示另一个字段的答案等于指定字符串，且该字段当前可见。
//
// 字段可见必须有从无条件字段出发的有限证明（最小不动点语义）：
// 仅靠环内字段互相满足条件不能让环自行显示，
// 外部支撑被撤销后环内字段也不能沿用旧的可见状态。
package formflow

import "fmt"

// Condition 表示一个条件：字段 Field 的原始答案等于 Equals，
// 且 Field 当前可见。
type Condition struct {
	Field  string
	Equals string
}

// ConditionGroup 是一组条件的合取：组内所有条件都满足时该组成立。
type ConditionGroup []Condition

// FieldDef 描述一个字段及其显隐规则。
// Groups 为空表示该字段无条件可见。
type FieldDef struct {
	Name   string
	Groups []ConditionGroup
}

// Definition 是一份问卷定义。输入保证所有条件引用的字段都存在。
type Definition struct {
	Fields []FieldDef
}

// Compiled 是编译后的定义，可安全地供多个会话并发复用。
type Compiled struct {
	fields []compiledField
	index  map[string]int
}

type compiledField struct {
	name   string
	groups [][]compiledCondition
}

type compiledCondition struct {
	field  int
	equals string
}

// Compile 编译一份定义，结果可供多个会话复用。
func Compile(def Definition) (*Compiled, error) {
	c := &Compiled{
		fields: make([]compiledField, len(def.Fields)),
		index:  make(map[string]int, len(def.Fields)),
	}
	for i, f := range def.Fields {
		if f.Name == "" {
			return nil, fmt.Errorf("formflow: field %d has empty name", i)
		}
		if _, dup := c.index[f.Name]; dup {
			return nil, fmt.Errorf("formflow: duplicate field %q", f.Name)
		}
		c.index[f.Name] = i
		c.fields[i].name = f.Name
	}
	for i, f := range def.Fields {
		groups := make([][]compiledCondition, len(f.Groups))
		for j, g := range f.Groups {
			conds := make([]compiledCondition, len(g))
			for k, cond := range g {
				ref, ok := c.index[cond.Field]
				if !ok {
					return nil, fmt.Errorf("formflow: field %q references unknown field %q", f.Name, cond.Field)
				}
				conds[k] = compiledCondition{field: ref, equals: cond.Equals}
			}
			groups[j] = conds
		}
		c.fields[i].groups = groups
	}
	return c, nil
}

// Change 表示对单个字段的一次赋值或清除。
type Change struct {
	Field string
	// Set 为 true 表示赋值为 Value（可以是空字符串），
	// 为 false 表示清除（回到未填写状态）。
	Set   bool
	Value string
}

// FieldDelta 描述一个字段在一次 Patch 后的状态变化。
// 只有可见性或有效答案发生变化的字段才会出现在 Patch 结果中。
type FieldDelta struct {
	Field string
	// Visible 为变化后的可见性。
	Visible bool
	// 以下字段描述变化后的有效答案。
	// 字段不可见或答案未填写时 Filled 为 false。
	Filled bool
	Value  string
}

package formflow

import "fmt"

// Compiled 是 Definition 的编译结果，可供多个会话并发复用。
type Compiled struct {
	fields []FieldDef
	index  map[string]int
}

// Compile 编译一份定义，结果可复用于多个会话。
// 字段名重复或条件引用了不存在的字段时返回错误。
func Compile(def Definition) (*Compiled, error) {
	c := &Compiled{
		fields: make([]FieldDef, len(def.Fields)),
		index:  make(map[string]int, len(def.Fields)),
	}
	for i, f := range def.Fields {
		if f.Name == "" {
			return nil, fmt.Errorf("formflow: 第 %d 个字段名为空", i)
		}
		if _, dup := c.index[f.Name]; dup {
			return nil, fmt.Errorf("formflow: 字段 %q 重复定义", f.Name)
		}
		c.fields[i] = f
		c.index[f.Name] = i
	}
	for _, f := range c.fields {
		for _, group := range f.Groups {
			for _, cond := range group {
				if _, ok := c.index[cond.Field]; !ok {
					return nil, fmt.Errorf("formflow: 字段 %q 的条件引用了不存在的字段 %q", f.Name, cond.Field)
				}
			}
		}
	}
	return c, nil
}

// Session 表示一次独立的填写会话，保留所有字段的原始答案（含隐藏字段）。
type Session struct {
	c       *Compiled
	answers map[string]string // 原始答案；key 存在表示已填写（可为空字符串）
	visible []bool
}

// Open 用编译结果和初始答案创建独立会话，并计算初始可见性。
// initial 中 key 存在即视为已填写，值可以是空字符串；未知字段被忽略。
func Open(c *Compiled, initial map[string]string) *Session {
	s := &Session{
		c:       c,
		answers: make(map[string]string, len(initial)),
	}
	for name, value := range initial {
		if _, ok := c.index[name]; ok {
			s.answers[name] = value
		}
	}
	s.visible = evaluate(c, s.answers)
	return s
}

// IsVisible 返回字段当前是否可见。
func (s *Session) IsVisible(field string) bool {
	i, ok := s.c.index[field]
	return ok && s.visible[i]
}

// Answers 返回有效答案视图：仅包含当前可见且已填写字段的答案。
func (s *Session) Answers() map[string]string {
	view := make(map[string]string, len(s.answers))
	for i, f := range s.c.fields {
		if s.visible[i] {
			if value, ok := s.answers[f.Name]; ok {
				view[f.Name] = value
			}
		}
	}
	return view
}

// RawAnswers 返回会话保留的全部原始答案，包括隐藏字段。
func (s *Session) RawAnswers() map[string]string {
	raw := make(map[string]string, len(s.answers))
	for name, value := range s.answers {
		raw[name] = value
	}
	return raw
}

// Patch 一次赋值或清除多个字段，按整批修改后的答案重新计算可见性，
// 返回可见性或有效答案发生变化的字段。批内处理顺序不影响结果。
// 同一字段在一批内只应出现一次；若重复出现，以最后一次操作为准。
func (s *Session) Patch(ops ...Op) Diff {
	before := s.snapshot()

	for _, op := range ops {
		if _, ok := s.c.index[op.Field]; !ok {
			continue
		}
		if op.Set {
			s.answers[op.Field] = op.Value
		} else {
			delete(s.answers, op.Field)
		}
	}
	s.visible = evaluate(s.c, s.answers)

	after := s.snapshot()
	diff := Diff{}
	for name, newState := range after {
		if before[name] != newState {
			diff[name] = newState
		}
	}
	return diff
}

// snapshot 返回每个字段当前的有效状态（可见性 + 有效答案）。
func (s *Session) snapshot() map[string]FieldState {
	states := make(map[string]FieldState, len(s.c.fields))
	for i, f := range s.c.fields {
		state := FieldState{Visible: s.visible[i]}
		if state.Visible {
			if value, ok := s.answers[f.Name]; ok {
				state.Answered = true
				state.Answer = value
			}
		}
		states[f.Name] = state
	}
	return states
}

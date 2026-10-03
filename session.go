package formflow

// Session 是一次独立的填写会话。
// 会话保留所有字段的原始答案（包括隐藏字段），
// 但有效答案视图只包含可见且已填写的字段。
type Session struct {
	compiled *Compiled
	raw      []rawAnswer
	visible  []bool
}

type rawAnswer struct {
	filled bool
	value  string
}

// Open 用编译结果和初始答案创建独立会话，计算初始可见性及有效答案。
// 初始答案中不属于定义的字段会被忽略。
func Open(c *Compiled, initial map[string]string) *Session {
	s := &Session{
		compiled: c,
		raw:      make([]rawAnswer, len(c.fields)),
	}
	for name, value := range initial {
		if i, ok := c.index[name]; ok {
			s.raw[i] = rawAnswer{filled: true, value: value}
		}
	}
	s.visible = s.evaluate()
	return s
}

// Visible 报告字段当前是否可见。
func (s *Session) Visible(field string) bool {
	if i, ok := s.compiled.index[field]; ok {
		return s.visible[i]
	}
	return false
}

// RawAnswers 返回所有字段的原始答案（仅含已填写的字段），
// 包括当前隐藏的字段。
func (s *Session) RawAnswers() map[string]string {
	out := make(map[string]string, len(s.raw))
	for i, a := range s.raw {
		if a.filled {
			out[s.compiled.fields[i].name] = a.value
		}
	}
	return out
}

// EffectiveAnswers 返回有效答案视图：仅包含可见且已填写字段的答案。
func (s *Session) EffectiveAnswers() map[string]string {
	out := make(map[string]string, len(s.raw))
	for i, a := range s.raw {
		if a.filled && s.visible[i] {
			out[s.compiled.fields[i].name] = a.value
		}
	}
	return out
}

// Patch 一次赋值或清除多个字段，返回可见性或有效答案发生变化的字段。
// 结果按整批修改后的答案决定，与批内字段的处理顺序无关。
// 仅原始隐藏答案变化（可见性与有效答案均未变）的字段不会出现在结果中。
func (s *Session) Patch(changes []Change) []FieldDelta {
	type state struct {
		visible bool
		filled  bool
		value   string
	}
	before := make([]state, len(s.raw))
	for i, a := range s.raw {
		before[i] = state{visible: s.visible[i], filled: a.filled, value: a.value}
	}
	for _, ch := range changes {
		i, ok := s.compiled.index[ch.Field]
		if !ok {
			continue
		}
		if ch.Set {
			s.raw[i] = rawAnswer{filled: true, value: ch.Value}
		} else {
			s.raw[i] = rawAnswer{}
		}
	}
	s.visible = s.evaluate()
	var deltas []FieldDelta
	for i := range s.raw {
		after := state{visible: s.visible[i], filled: s.raw[i].filled, value: s.raw[i].value}
		if !after.visible {
			after.filled, after.value = false, ""
		}
		prev := before[i]
		if !prev.visible {
			prev.filled, prev.value = false, ""
		}
		if prev == after {
			continue
		}
		deltas = append(deltas, FieldDelta{
			Field:   s.compiled.fields[i].name,
			Visible: after.visible,
			Filled:  after.filled,
			Value:   after.value,
		})
	}
	return deltas
}

// evaluate 以最小不动点语义计算可见性：
// 从全隐藏出发，反复加入“无条件可见”或“某一条件组全部满足”的字段，
// 直到不再变化。因此可见性一定有从无条件字段出发的有限证明，
// 环内字段无法仅靠互相满足条件而自行显示。
func (s *Session) evaluate() []bool {
	visible := make([]bool, len(s.compiled.fields))
	for changed := true; changed; {
		changed = false
		for i, f := range s.compiled.fields {
			if visible[i] {
				continue
			}
			if len(f.groups) == 0 {
				visible[i] = true
				changed = true
				continue
			}
			for _, g := range f.groups {
				ok := true
				for _, cond := range g {
					a := s.raw[cond.field]
					if !visible[cond.field] || !a.filled || a.value != cond.equals {
						ok = false
						break
					}
				}
				if ok {
					visible[i] = true
					changed = true
					break
				}
			}
		}
	}
	return visible
}

package formflow

// evaluate 计算给定原始答案下的最小不动点可见集合：
// 从无规则字段出发，反复把“某一条件组全部满足”的字段加入可见集，
// 直到不再变化。仅靠环内字段互相满足条件无法让环自行显示。
func evaluate(c *Compiled, answers map[string]string) []bool {
	visible := make([]bool, len(c.fields))
	for i, f := range c.fields {
		if len(f.Groups) == 0 {
			visible[i] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for i, f := range c.fields {
			if visible[i] {
				continue
			}
			if groupsSatisfied(c, f.Groups, answers, visible) {
				visible[i] = true
				changed = true
			}
		}
	}
	return visible
}

func groupsSatisfied(c *Compiled, groups [][]Condition, answers map[string]string, visible []bool) bool {
	for _, group := range groups {
		if conditionSatisfied(c, group, answers, visible) {
			return true
		}
	}
	return false
}

func conditionSatisfied(c *Compiled, group []Condition, answers map[string]string, visible []bool) bool {
	for _, cond := range group {
		j, ok := c.index[cond.Field]
		if !ok || !visible[j] {
			return false
		}
		answer, answered := answers[cond.Field]
		if !answered || answer != cond.Equals {
			return false
		}
	}
	return true
}

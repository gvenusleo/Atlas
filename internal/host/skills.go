package host

import "github.com/gvenusleo/atlas/internal/skills"

// DiscoverSkills 发现工作目录下可用的技能。会话创建或恢复之前，界面用它列出可以调用的技能；
// 会话存在以后，以会话自己的快照（State.Skills）为准。
func DiscoverSkills(cwd string) ([]skills.Skill, error) {
	return skills.Discover(cwd)
}

package gui

import (
	"cmp"
	"errors"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/gvenusleo/atlas/internal/config"
)

// 这个文件是设置页“模型接入”一类的状态与逻辑：读写 ~/.atlas/settings.json 与 auth.json
// （见 config），界面在 modelsview.go。

// thinkingLevels 是思考深度可选的档位，由浅到深。它与 agent 的档位词表一致；gui 不依赖 agent，
// 所以这里再列一遍。
var thinkingLevels = []string{"minimal", "low", "medium", "high", "xhigh", "max"}

// modelsPage 是“模型接入”一类的状态。
type modelsPage struct {
	cfg     *config.Settings // 打开设置页时读到的，之后每次保存成功才换成新的；读不了时为空
	stat    fileStat         // settings.json 现在的样子，用来发现别处的修改
	keys    map[string]bool  // 各服务在 auth.json 里有没有 key
	pick    string           // 默认模型下拉里选中的引用，空表示自动
	loadErr string           // cfg 为空时，读不了的原因
	saveErr string           // 最近一次保存失败的原因
	form    *serviceForm     // 打开着的服务对话框
}

// loadModels 读入 settings.json 与各服务的 key 状态。文件不存在算零配置；读不了或校验不过时页面
// 只显示原因，不让改，免得盖掉一个读不懂的文件。文件很小，在主线程同步读。
func (a *App) loadModels() {
	p := &a.settings.models
	dir, err := config.Dir()
	if err != nil {
		p.loadErr = err.Error()
		return
	}
	cfg, err := config.LoadFrom(dir)
	if err != nil {
		p.loadErr = err.Error()
		return
	}
	p.cfg, p.stat, p.pick = cfg, settingsStat(dir), cfg.Model
	p.refreshKeys(dir)
}

// settingsStat 是 dir 下 settings.json 现在的样子；没有这个文件时是零值。
func settingsStat(dir string) fileStat {
	info, err := os.Stat(filepath.Join(dir, config.SettingsFile))
	if err != nil {
		return fileStat{}
	}
	return fileStat{info.ModTime(), info.Size()}
}

// refreshKeys 重新查各服务在 auth.json 里有没有 key。
func (p *modelsPage) refreshKeys(dir string) {
	p.keys = map[string]bool{}
	for name := range p.cfg.Providers {
		_, err := config.APIKey(dir, name)
		p.keys[name] = err == nil
	}
}

// providerNames 是全部服务的名称，按字母排。
func (p *modelsPage) providerNames() []string {
	return slices.Sorted(maps.Keys(p.cfg.Providers))
}

// modelRefs 是默认模型下拉的各项：第一项空串是“自动”，其后是声明过的全部模型的引用。
func (p *modelsPage) modelRefs() []string {
	refs := []string{""}
	for _, name := range p.providerNames() {
		for _, id := range slices.Sorted(maps.Keys(p.cfg.Providers[name].Models)) {
			refs = append(refs, name+"/"+id)
		}
	}
	return refs
}

// modelLabel 是模型引用在下拉里的显示：服务名加模型的展示名。
func (p *modelsPage) modelLabel(ref string) string {
	if ref == "" {
		return "自动"
	}
	provider, id, err := config.ParseModel(ref)
	if err != nil {
		return ref
	}
	return provider + " / " + cmp.Or(p.cfg.Providers[provider].Models[id].Name, id)
}

// commitModels 在设置的副本上做 edit 再保存；保存成功才换上副本。磁盘上的文件在读入之后被别处
// 改过时不覆盖。
func (a *App) commitModels(edit func(next *config.Settings)) error {
	p := &a.settings.models
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	info, err := os.Stat(filepath.Join(dir, config.SettingsFile))
	if err == nil && !p.stat.same(info) {
		return errors.New("settings.json 在磁盘上已变更，请重新打开设置页")
	}
	next := *p.cfg
	next.Providers = maps.Clone(p.cfg.Providers)
	if next.Providers == nil {
		next.Providers = map[string]config.Provider{}
	}
	edit(&next)
	if err := config.Save(dir, &next); err != nil {
		return err
	}
	p.cfg, p.stat, p.pick = &next, settingsStat(dir), next.Model
	a.refreshOptions()
	return nil
}

// chooseDefaultModel 在默认模型下拉的选择变了之后保存。
func (a *App) chooseDefaultModel() {
	p := &a.settings.models
	pick := p.pick
	p.saveErr = ""
	if err := a.commitModels(func(next *config.Settings) { next.Model = pick }); err != nil {
		p.saveErr = "保存失败：" + err.Error()
		p.pick = p.cfg.Model
	}
}

// serviceForm 是新建或编辑模型服务的表单。
type serviceForm struct {
	orig          string // 编辑的服务原来的名称，新建为空
	name          string
	api           config.WireAPI
	apis          []config.WireAPI // 接口类型的各个选项
	baseURL       string
	key           string // API key 框里的内容，编辑时一开始是现有的 key
	key0          string // 打开时读到的 key，没读到为空
	showKey       bool   // API key 框显示明文；平时遮蔽
	models        []*modelForm
	next          int  // 给新加的模型行取不重复的键
	confirmDelete bool // 点过一次“删除服务”，再点一次才删
	err           string
}

// modelForm 是服务表单里的一行模型。
type modelForm struct {
	key     int
	open    bool   // 展开着：显示全部字段；收起时只有一行摘要
	origID  string // 已有模型原来的 id，新加的行为空；id 改了才知道它改自哪个
	id      string
	name    string
	context string // 上下文窗口，token 数
	output  string // 单次回复的输出上限，token 数
	images  bool
	levels  []bool       // 与 thinkingLevels 一一对应：这个模型支持哪些思考档位
	orig    config.Model // 原来的样子，界面里没有的字段原样带回去
	images0 bool
	levels0 []bool
}

// newServiceForm 返回新建服务的空表单。
func newServiceForm() *serviceForm {
	return &serviceForm{
		api:  config.WireAPIOpenAIResponses,
		apis: []config.WireAPI{config.WireAPIOpenAIResponses},
	}
}

// formOfService 返回编辑服务 name 的表单。接口类型的选项除了已经实现的协议，还有这个服务原来
// 用的，免得保存时把它换掉。
func formOfService(name string, prov config.Provider) *serviceForm {
	f := newServiceForm()
	f.orig, f.name, f.baseURL = name, name, prov.BaseURL
	if prov.WireAPI != "" {
		f.api = prov.WireAPI
		if !slices.Contains(f.apis, prov.WireAPI) {
			f.apis = append(f.apis, prov.WireAPI)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(prov.Models)) {
		f.models = append(f.models, f.modelOf(id, prov.Models[id]))
	}
	return f
}

// modelOf 返回已有模型 id 的一行。
func (f *serviceForm) modelOf(id string, m config.Model) *modelForm {
	row := f.blankModel()
	row.open, row.origID, row.id, row.name, row.orig = false, id, id, m.Name, m
	if m.ContextWindow > 0 {
		row.context = strconv.FormatInt(m.ContextWindow, 10)
	}
	if m.MaxOutput > 0 {
		row.output = strconv.FormatInt(m.MaxOutput, 10)
	}
	row.images = m.Modalities.AcceptsImages()
	for _, option := range m.ReasoningOptions {
		if option.Type != config.ReasoningTypeEffort {
			continue
		}
		for _, v := range option.Values {
			if i := slices.Index(thinkingLevels, v); i >= 0 && m.Reasoning {
				row.levels[i] = true
			}
		}
	}
	row.images0, row.levels0 = row.images, slices.Clone(row.levels)
	return row
}

// blankModel 返回一行新的、什么也没填的模型。
func (f *serviceForm) blankModel() *modelForm {
	f.next++
	return &modelForm{
		key: f.next, open: true, levels: make([]bool, len(thinkingLevels)),
		levels0: make([]bool, len(thinkingLevels)),
	}
}

// renamedModels 是 id 改过的已有模型：原来的 id 对应现在的 id。
func (f *serviceForm) renamedModels() map[string]string {
	renames := map[string]string{}
	for _, row := range f.models {
		if id := strings.TrimSpace(row.id); row.origID != "" && id != row.origID {
			renames[row.origID] = id
		}
	}
	return renames
}

// removeModel 从表单里去掉一行模型。
func (f *serviceForm) removeModel(m *modelForm) {
	f.models = slices.DeleteFunc(f.models, func(row *modelForm) bool { return row == m })
}

// model 返回这一行填成的模型：界面里有的字段取表单的值，其余照原样；没改过的项（图片、思考
// 档位）不动原来的写法。
func (m *modelForm) model() (config.Model, error) {
	out := m.orig
	out.Name = strings.TrimSpace(m.name)
	var err error
	if out.ContextWindow, err = parseTokens(m.context, "上下文窗口", m.id); err != nil {
		return config.Model{}, err
	}
	if out.MaxOutput, err = parseTokens(m.output, "最大输出", m.id); err != nil {
		return config.Model{}, err
	}
	if m.images != m.images0 {
		out.Modalities.Input = withImage(out.Modalities.Input, m.images)
	}
	if !slices.Equal(m.levels, m.levels0) {
		withLevels(&out, m.levels)
	}
	return out, nil
}

// parseTokens 把输入框里的 token 数读成整数：空表示 0，即不设；其余要是非负整数。what 与模型 id
// 用于报错。
func parseTokens(text, what, id string) (int64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil || n < 0 {
		return 0, errors.New("模型 " + id + " 的" + what + "要填非负整数")
	}
	return n, nil
}

// withImage 返回在输入模态 in 里加上或去掉图片之后的新切片。
func withImage(in []config.Modality, on bool) []config.Modality {
	in = slices.DeleteFunc(slices.Clone(in), func(m config.Modality) bool {
		return m == config.ModalityImage
	})
	if !on {
		return in
	}
	if len(in) == 0 {
		in = []config.Modality{config.ModalityText}
	}
	return append(in, config.ModalityImage)
}

// withLevels 把模型的思考档位换成 levels 选中的；其他种类的思考选项原样保留。
func withLevels(m *config.Model, levels []bool) {
	isEffort := func(o config.ReasoningOption) bool { return o.Type == config.ReasoningTypeEffort }
	options := slices.DeleteFunc(slices.Clone(m.ReasoningOptions), isEffort)
	var values []string
	for i, on := range levels {
		if on {
			values = append(values, thinkingLevels[i])
		}
	}
	if len(values) > 0 {
		effort := config.ReasoningOption{Type: config.ReasoningTypeEffort, Values: values}
		options = append([]config.ReasoningOption{effort}, options...)
	}
	m.ReasoningOptions = options
	m.Reasoning = len(options) > 0
}

// provider 校验表单并返回服务的名称与内容；cfg 是现有的设置，用来查重名。
func (f *serviceForm) provider(cfg *config.Settings) (string, config.Provider, error) {
	bad := func(msg string) (string, config.Provider, error) {
		return "", config.Provider{}, errors.New(msg)
	}
	name := strings.TrimSpace(f.name)
	switch {
	case name == "":
		return bad("名称不能为空")
	case strings.Contains(name, "/"):
		return bad("名称不能含有 /")
	}
	if _, exists := cfg.Providers[name]; exists && name != f.orig {
		return bad("已经有名为 " + name + " 的服务")
	}
	base := strings.TrimSpace(f.baseURL)
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return bad("接口地址要以 http:// 或 https:// 开头")
	}
	prov := config.Provider{WireAPI: f.api, BaseURL: base, Models: map[string]config.Model{}}
	for _, row := range f.models {
		id := strings.TrimSpace(row.id)
		if id == "" {
			return bad("有一个模型没有填 ID")
		}
		if _, dup := prov.Models[id]; dup {
			return bad("模型 ID " + id + " 重复了")
		}
		model, err := row.model()
		if err != nil {
			return bad(err.Error())
		}
		prov.Models[id] = model
	}
	return name, prov, nil
}

// saveService 保存服务表单。先写新的 key，再写 settings.json，最后才删不要的 key：中途失败最多
// 多出一条没人用的 key，不会出现有服务而没有 key，也不会设置没保存、key 却先丢了。
func (a *App) saveService() {
	p := &a.settings.models
	f := p.form
	name, prov, err := f.provider(p.cfg)
	if err != nil {
		f.err = err.Error()
		return
	}
	dir, err := config.Dir()
	if err != nil {
		f.err = err.Error()
		return
	}
	renamed := f.orig != "" && f.orig != name
	if err := f.writeKey(dir, name, renamed); err != nil {
		f.err = "保存 API key 失败：" + err.Error()
		return
	}
	renames := f.renamedModels()
	err = a.commitModels(func(next *config.Settings) {
		if renamed {
			delete(next.Providers, f.orig)
			if rest, ok := strings.CutPrefix(next.Model, f.orig+"/"); ok {
				next.Model = name + "/" + rest
			}
		}
		next.Providers[name] = prov
		// 默认模型指向这个服务里改了 id 的模型时跟着改；指向已经删掉的模型时，改回自动。
		if provider, id, err := config.ParseModel(next.Model); err == nil && provider == name {
			if to, ok := renames[id]; ok {
				next.Model, id = name+"/"+to, to
			}
			if _, ok := prov.Models[id]; !ok {
				next.Model = ""
			}
		}
	})
	if err != nil {
		f.err = "保存失败：" + err.Error()
		return
	}
	if stale := f.staleKey(name, renamed); stale != "" {
		if err := config.RemoveAPIKey(dir, stale); err != nil {
			a.toast = "没能删掉 API key：" + err.Error()
		}
	}
	p.refreshKeys(dir)
	p.form = nil
}

// writeKey 把框里改过的 API key 写进 auth.json。框里一开始是打开时读到的 key（key0）：没动或
// 清空了就不写；改名时新名下总要写上它。
func (f *serviceForm) writeKey(dir, name string, renamed bool) error {
	key := strings.TrimSpace(f.key)
	if key == "" || (key == f.key0 && !renamed) {
		return nil
	}
	return config.SetAPIKey(dir, name, key)
}

// staleKey 是保存之后该从 auth.json 里删掉的 key 所属的服务，没有时为空：改名时是旧名，清空了
// 框时是它自己。打开时读不到旧 key、又没填新的，就不去动它，免得删掉一个看不见的 key。
func (f *serviceForm) staleKey(name string, renamed bool) string {
	key := strings.TrimSpace(f.key)
	switch {
	case renamed && (f.key0 != "" || key != ""):
		return f.orig
	case !renamed && key == "" && f.key0 != "":
		return name
	}
	return ""
}

// editService 打开服务 name 的表单，API key 框里是它现在的 key（读得到的话）。key 来自
// auth.json，框里遮蔽显示，不进日志。
func (a *App) editService(name string) {
	p := &a.settings.models
	f := formOfService(name, p.cfg.Providers[name])
	if dir, err := config.Dir(); err == nil {
		if key, err := config.APIKey(dir, name); err == nil {
			f.key, f.key0 = key, key
		}
	}
	p.form = f
}

// deleteService 删掉正在编辑的服务和它的 key。点第一次只是要求确认。
func (a *App) deleteService() {
	p := &a.settings.models
	f := p.form
	if !f.confirmDelete {
		f.confirmDelete = true
		return
	}
	err := a.commitModels(func(next *config.Settings) {
		delete(next.Providers, f.orig)
		if strings.HasPrefix(next.Model, f.orig+"/") {
			next.Model = ""
		}
	})
	if err != nil {
		f.err = "删除失败：" + err.Error()
		return
	}
	dir, err := config.Dir()
	if err == nil {
		err = config.RemoveAPIKey(dir, f.orig)
		p.refreshKeys(dir)
	}
	if err != nil {
		a.toast = "没能删掉 API key：" + err.Error()
	}
	p.form = nil
}

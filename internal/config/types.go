// Package config 定义与校验仓库流水线，加载阶段不解析密钥或执行命令。
package config

const (
	MaxConfigBytes = 1 << 20
	MaxDepth       = 32
	MaxNodes       = 10000
)

// Document 将旧根级配置统一规范化为名为 default 的构建。
type Document struct {
	Version       int
	Builds        map[string]*Build
	Notifications *Notifications
}

type Build struct {
	IOSSigning    *IOSSigning          `yaml:"ios_signing,omitempty" json:",omitempty"`
	Runner        *Runner              `yaml:"runner,omitempty"`
	Params        map[string]Parameter `yaml:"params,omitempty"`
	Env           map[string]string    `yaml:"env,omitempty"`
	When          *When                `yaml:"when,omitempty"`
	Timeout       string               `yaml:"timeout,omitempty"`
	Steps         []Step               `yaml:"steps"`
	Post          *Post                `yaml:"post,omitempty"`
	Reports       *Reports             `yaml:"reports,omitempty"`
	Notifications *Notifications       `yaml:"notifications,omitempty"`
}

type Runner struct {
	Framework string   `yaml:"framework,omitempty" json:"framework,omitempty"`
	Platform  string   `yaml:"platform"`
	Labels    []string `yaml:"labels,omitempty"`
}

type Parameter struct {
	Default     *string  `yaml:"default,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Required    bool     `yaml:"required,omitempty"`
	Choices     []string `yaml:"choices,omitempty"`
}

type When struct {
	Branches []string          `yaml:"branches,omitempty"`
	Params   map[string]string `yaml:"params,omitempty"`
	Changes  []string          `yaml:"changes,omitempty"`
}

// Step 的各字段仅用于所属 kind，严格检查由解析/校验函数完成。
type Step struct {
	Kind             string            `yaml:"kind"`
	Name             string            `yaml:"name,omitempty"`
	When             *When             `yaml:"when,omitempty"`
	Run              string            `yaml:"run,omitempty"`
	Shell            string            `yaml:"shell,omitempty"`
	WorkingDir       string            `yaml:"working_dir,omitempty"`
	Env              map[string]string `yaml:"env,omitempty"`
	Timeout          string            `yaml:"timeout,omitempty"`
	Paths            []string          `yaml:"paths,omitempty"`
	Notify           *bool             `yaml:"notify,omitempty"`
	Target           string            `yaml:"target,omitempty"`
	File             string            `yaml:"file,omitempty"`
	Channel          string            `yaml:"channel,omitempty"`
	Track            string            `yaml:"track,omitempty"`
	Credentials      string            `yaml:"credentials,omitempty"`
	AppIdentifier    string            `yaml:"app_identifier,omitempty"`
	ReleaseStatus    string            `yaml:"release_status,omitempty"`
	SubmitForReview  *bool             `yaml:"submit_for_review,omitempty"`
	AutomaticRelease *bool             `yaml:"automatic_release,omitempty"`
	Argv             []string          `yaml:"argv,omitempty"`
	ResultFile       string            `yaml:"result_file,omitempty"`
	QueryArgv        []string          `yaml:"query_argv,omitempty"`
}

type Post struct {
	Timeout string `yaml:"timeout,omitempty"`
	Success []Step `yaml:"success,omitempty"`
	Failure []Step `yaml:"failure,omitempty"`
	Always  []Step `yaml:"always,omitempty"`
}

type Reports struct {
	JUnit *JUnitReport `yaml:"junit,omitempty"`
}

type JUnitReport struct {
	Paths    []string `yaml:"paths"`
	Required *bool    `yaml:"required,omitempty"`
	MaxFiles *int     `yaml:"max_files,omitempty" json:",omitempty"`
}

const DefaultJUnitMaxFiles = 256
const MaximumJUnitMaxFiles = 1024

// FileLimit读取冻结配置，省略时使用默认；合法范围由Validate检查。
func (r *JUnitReport) FileLimit() int {
	if r == nil || r.MaxFiles == nil {
		return DefaultJUnitMaxFiles
	}
	return *r.MaxFiles
}

type Notifications struct {
	Enabled  *bool     `yaml:"enabled,omitempty"`
	On       []string  `yaml:"on,omitempty"`
	Webhooks []Webhook `yaml:"webhooks,omitempty"`
	Template string    `yaml:"template,omitempty"`
}

type Webhook struct {
	Type string `yaml:"type"`
	URL  string `yaml:"url"`
}

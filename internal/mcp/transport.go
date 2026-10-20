package mcp

import (
	"cmp"
	"net/http"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gvenusleo/atlas/internal/shellenv"
)

// stderrTailBytes 是失败时附在原因后面的子进程 stderr 的长度。
const stderrTailBytes = 2000

// conn 是一次连接尝试用到的传输；stdio 时还有子进程与记录它 stderr 末尾的缓冲。
type conn struct {
	transport sdk.Transport
	cmd       *exec.Cmd
	stderr    *tailBuffer
}

// open 按配置建立传输。配置里的值都按字面使用，不展开环境变量。
func (s *server) open() (conn, error) {
	if s.cfg.URL != "" {
		return s.httpConn(), nil
	}
	return s.stdioConn()
}

// stdioConn 准备 stdio 子进程：环境是登录 shell 的环境加上配置的，工作目录缺省为主目录。
func (s *server) stdioConn() (conn, error) {
	argv := slices.Concat([]string{s.cfg.Command}, s.cfg.Args)
	for i, arg := range argv {
		var err error
		if argv[i], err = expandHome(arg); err != nil {
			return conn{}, err
		}
	}
	dir, err := expandHome(cmp.Or(s.cfg.Cwd, "~"))
	if err != nil {
		return conn{}, err
	}
	env := shellenv.Environ()
	for key, value := range s.cfg.Env {
		env = append(env, key+"="+value)
	}
	file, err := lookPath(argv[0], env)
	if err != nil {
		return conn{}, err
	}
	cmd := exec.Command(file, argv[1:]...)
	cmd.Dir, cmd.Env = dir, env
	stderr := &tailBuffer{}
	cmd.Stderr = stderr
	setGroup(cmd)
	return conn{transport: &sdk.CommandTransport{Command: cmd}, cmd: cmd, stderr: stderr}, nil
}

// lookPath 在 env 的 PATH 里找可执行文件。exec.Command 只在 Atlas 自己的 PATH 里找，而从
// Dock 启动的 Atlas 没有 shell 配置里设置的 PATH，找不到 npx、uvx；env 里没有就退回那个 PATH。
func lookPath(name string, env []string) (string, error) {
	if filepath.Base(name) != name {
		return name, nil // 带目录的路径照用
	}
	var path string
	for _, kv := range env {
		if value, ok := strings.CutPrefix(kv, "PATH="); ok {
			path = value // 重复时以最后一个为准
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		if file, err := exec.LookPath(filepath.Join(dir, name)); err == nil {
			return file, nil
		}
	}
	return exec.LookPath(name)
}

// httpConn 准备 Streamable HTTP 传输；配置了请求头时，每个请求都带上。
func (s *server) httpConn() conn {
	var client *http.Client
	if len(s.cfg.Headers) > 0 {
		header := http.Header{}
		for key, value := range s.cfg.Headers {
			header.Set(key, value)
		}
		client = &http.Client{Transport: headerTransport{header}}
	}
	transport := &sdk.StreamableClientTransport{Endpoint: s.cfg.URL, HTTPClient: client}
	return conn{transport: transport}
}

// headerTransport 给每个请求加上配置里的请求头。
type headerTransport struct{ header http.Header }

func (h headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for key, values := range h.header {
		r.Header[key] = values
	}
	return http.DefaultTransport.RoundTrip(r)
}

// authHint 在 HTTP 服务器拒绝了未授权的请求时，补一句办法：Atlas 不支持 OAuth 登录。SDK 没有
// 给这类失败单独的错误类型，只能按状态文字判断；认不出来时只是少了这句提示。
func authHint(err error) string {
	msg := err.Error()
	if !strings.Contains(msg, http.StatusText(http.StatusUnauthorized)) &&
		!strings.Contains(msg, http.StatusText(http.StatusForbidden)) {
		return ""
	}
	return "\n服务器要求授权；Atlas 暂不支持 OAuth 登录，可在 headers 里放访问令牌，" +
		`如 "Authorization": "Bearer 令牌"`
}

// tailBuffer 只留最后写入的 stderrTailBytes 个字节。
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if extra := len(t.buf) - stderrTailBytes; extra > 0 {
		t.buf = t.buf[extra:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.ToValidUTF8(string(t.buf), "")
}

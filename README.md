# Atlas

Atlas 正在从一个干净的 Go module 重新开始开发。

## 配置

Atlas 从用户主目录下的 `.atlas/config.json` 读取 Provider 配置：

```json
{
  "provider": {
    "base_url": "https://api.deepseek.com",
    "api_key": "sk-...",
    "model": "deepseek-v4-flash"
  },
  "agent": {
    "max_steps": 8,
    "temperature": 0.2
  }
}
```

Atlas 默认拥有当前进程的完整本地访问权限，工具可以读写文件并执行 shell 命令。

## 使用

```sh
go run ./cmd/atlas "读取 README 并总结"
```

## 开发

```sh
go test ./...
```

---
name: release
description: 发布 Atlas 新版本：更新版本号和更新日志，运行项目检查，提交并推送版本标签，等待 GitHub Actions 构建后公开 Release。
when_to_use: 用户要求发布 Atlas 新版本、发版或创建正式 Release 时使用。
---

# 发布 Atlas

按仓库现有发布流程完成版本提交、跨平台构建和公开 Release。用户明确要求发版时，授权包含完成这条发布链路，无需在每一步再次询问。

## 流程

1. 确认工作区状态、当前分支、`origin/main` 和最新正式 GitHub Release。检查目标版本尚未发布，且高于当前版本。工作区有无关修改或与 `main` 不同步时，先停下说明情况，不覆盖或隐藏现有工作。
2. 从最新正式版本标签到 `HEAD` 查看提交和用户可见变化。将 `mygo.json` 的 `version` 更新为目标版本，在 `CHANGELOG.md` 顶部增加对应版本条目。按用户结果归纳条目，不写内部实现和发布流水线细节。保持更新日志的中文风格。
3. 运行 `mise run fmt`、`mise run ci`，并用 `git diff --check` 检查改动。任一失败时先说明失败，不提交或发布。
4. 只提交 `mygo.json` 和 `CHANGELOG.md`，提交说明使用仓库约定的中文格式 `build: 发布 X.Y.Z`。创建注释标签 `vX.Y.Z` 指向该提交，推送 `main` 和标签到 `origin`。
5. 等待 `.github/workflows/release.yml` 的运行完成。只有草稿创建及 macOS、Windows、Linux 三个构建任务全部成功，才继续发布。失败时保留草稿并说明失败任务。
6. 查看草稿 Release 的资产列表，确认有各平台构建产物、安装脚本、校验文件和六个平台更新清单。确认更新清单版本为目标版本，下载地址指向该 Release。无需逐个下载资产或重复计算工作流已生成的校验和。
7. 将草稿设为正式 Release，使用 `vX.Y.Z` 作为标题。重新读取 Release 状态，确认它已公开且不是预发布版；检查远端 `main`、标签和本地工作区状态。

## 项目发布配置

- 版本源为 `mygo.json` 的 `version`，标签格式为 `vX.Y.Z`。
- Release 工作流由推送 `v*` 标签触发，先创建草稿，再在 macOS、Windows、Linux 构建并上传产物。
- 工作流会检查标签与 `mygo.json` 版本一致。macOS 构建使用仓库配置的签名证书，更新包使用仓库配置的私钥。
- CI 工作流配置为 PR 触发。按仓库要求在发布提交前本地运行 `mise run ci`，不要把没有触发的推送 CI 说成已通过。
- Release 上传完成后由发布者公开草稿。

## 完成汇报

简要报告版本号、提交与标签、检查结果、三个构建任务状态和 Release 链接。只报告本轮实际读取到的状态。

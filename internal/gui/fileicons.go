package gui

import (
	"cmp"
	"path"
	"strings"

	"github.com/egoist/mygo/ui"
)

// fileIconNames 把小写的文件名或扩展名（带点）对应到 icons/material 里的图标名；
// 图标取自 Material Icon Theme，对应关系也参照它，只是 .pl 归 Perl、.ogg 归音频。
// 先按文件名、再按扩展名找。
var fileIconNames = map[string]string{
	// 项目与文档的文件名
	"go.mod": "go-mod", "go.sum": "go-mod", "license": "license", "license.md": "license",
	"license.txt": "license", "readme": "readme", "readme.md": "readme", "readme.txt": "readme",
	"changelog": "changelog", "changelog.md": "changelog", "changelog.rst": "changelog",
	"changelog.txt": "changelog", "changes": "changelog", "changes.md": "changelog",
	"changes.txt": "changelog", "contributing": "contributing", "contributing.md": "contributing",
	"contributing.rst": "contributing", "contributing.txt": "contributing", "authors": "authors",
	"authors.md": "authors", "authors.txt": "authors", "contributors": "authors",
	"contributors.md": "authors", "contributors.txt": "authors", "code_of_conduct": "conduct",
	"code_of_conduct.md": "conduct", "code_of_conduct.txt": "conduct", "credits": "credits",
	"credits.md": "credits", "credits.txt": "credits", "roadmap.md": "roadmap",
	"roadmap.txt": "roadmap", "robots.txt": "robots", "todo.md": "todo", "todos.md": "todo",
	"hosts": "hosts", "favicon.ico": "favicon", "codeowners": "codeowners", "owners": "codeowners",
	"claude.md": "claude", "claude.local.md": "claude", "agents.md": "agent",
	"skill.md": "skill", "skills.md": "skill", "makefile": "makefile", "tsconfig.json": "tsconfig",
	"package.json": "nodejs", "package-lock.json": "nodejs", ".nvmrc": "nodejs",
	".node-version": "nodejs", ".gitignore": "git", ".gitattributes": "git",
	".gitmodules": "git", ".env": "tune", ".env.local": "tune", ".env.example": "tune",
	".env.sample": "tune", ".env.development": "tune", ".env.production": "tune",

	// 构建、依赖与部署的文件名
	"dockerfile": "docker", "docker-compose.yml": "docker", "docker-compose.yaml": "docker",
	"compose.yml": "docker", "compose.yaml": "docker", ".dockerignore": "docker",
	"cmakelists.txt": "cmake", "cmakecache.txt": "cmake", "cmakepresets.json": "cmake",
	"gradle.properties": "gradle", "gradlew": "gradle", "gradlew.bat": "gradle",
	"gradle-wrapper.properties": "gradle", "pom.xml": "maven", "maven.config": "maven",
	"jvm.config": "maven", ".bazelrc": "bazel", ".bazelignore": "bazel", ".bazelversion": "bazel",
	"justfile": "just", ".justfile": "just", "gemfile": "gemfile", "poetry.lock": "poetry",
	"requirements.txt": "python-misc", "pipfile": "python-misc", ".python-version": "python-misc",
	"pyproject.toml": "python-misc", ".pylintrc": "python-misc", ".coveragerc": "python-misc",
	"androidmanifest.xml": "android", ".vsconfig": "visualstudio", "nuget.config": "nuget",
	"vagrantfile": "vagrant", "nginx.conf": "nginx", "k8s.yml": "kubernetes",
	"k8s.yaml": "kubernetes", "kubernetes.yml": "kubernetes", "kubernetes.yaml": "kubernetes",
	".helmignore": "helm", "circle.yml": "circleci", ".gitlab-ci.yml": "gitlab",
	"dependabot.yml": "dependabot", "dependabot.yaml": "dependabot", ".renovaterc": "renovate",
	".renovaterc.json": "renovate", "renovate.json": "renovate", "renovate.json5": "renovate",
	"vercel.json": "vercel", ".vercelignore": "vercel", "netlify.toml": "netlify",
	"netlify.json": "netlify", "netlify.yml": "netlify", ".codecov.yml": "codecov",
	".codecov.yaml": "codecov", "codecov.yml": "codecov", "codecov.yaml": "codecov",
	"openapi.json": "openapi", "openapi.yml": "openapi", "openapi.yaml": "openapi",
	"swagger.json": "swagger", "swagger.yml": "swagger", "swagger.yaml": "swagger",

	// 前端工具链的配置文件名
	"vite.config.js": "vite", "vite.config.mjs": "vite", "vite.config.cjs": "vite",
	"vite.config.ts": "vite", "vite.config.mts": "vite", "vitest.config.js": "vitest",
	"vitest.config.mjs": "vitest", "vitest.config.ts": "vitest", "vitest.config.mts": "vitest",
	".eslintrc": "eslint", ".eslintrc.json": "eslint", ".eslintrc.js": "eslint",
	".eslintrc.cjs": "eslint", ".eslintrc.yml": "eslint", ".eslintrc.yaml": "eslint",
	"eslint.config.js": "eslint", "eslint.config.mjs": "eslint", "eslint.config.cjs": "eslint",
	"eslint.config.ts": "eslint", ".eslintignore": "eslint", ".prettierrc": "prettier",
	".prettierrc.json": "prettier", ".prettierrc.js": "prettier", ".prettierrc.cjs": "prettier",
	".prettierrc.yml": "prettier", ".prettierrc.yaml": "prettier", ".prettierrc.toml": "prettier",
	"prettier.config.js": "prettier", "prettier.config.mjs": "prettier",
	"prettier.config.cjs": "prettier", ".prettierignore": "prettier", ".babelrc": "babel",
	".babelrc.json": "babel", ".babelrc.js": "babel", "babel.config.js": "babel",
	"babel.config.json": "babel", "babel.config.cjs": "babel", "babel.config.mjs": "babel",
	"webpack.config.js": "webpack", "webpack.config.ts": "webpack", "webpack.config.mjs": "webpack",
	"webpack.config.cjs": "webpack", "webpack.common.js": "webpack", "webpack.dev.js": "webpack",
	"webpack.prod.js": "webpack", "rollup.config.js": "rollup", "rollup.config.mjs": "rollup",
	"rollup.config.cjs": "rollup", "rollup.config.ts": "rollup", "jest.config.js": "jest",
	"jest.config.cjs": "jest", "jest.config.mjs": "jest", "jest.config.ts": "jest",
	"jest.config.json": "jest", "tailwind.config.js": "tailwindcss",
	"tailwind.config.cjs": "tailwindcss", "tailwind.config.mjs": "tailwindcss",
	"tailwind.config.ts": "tailwindcss", "postcss.config.js": "postcss",
	"postcss.config.cjs": "postcss", "postcss.config.mjs": "postcss", ".postcssrc": "postcss",
	"next.config.js": "next", "next.config.mjs": "next", "next.config.ts": "next",
	"nuxt.config.js": "nuxt", "nuxt.config.ts": "nuxt", "pnpm-lock.yaml": "pnpm",
	"pnpm-workspace.yaml": "pnpm", ".pnpmfile.cjs": "pnpm", "yarn.lock": "yarn",
	".yarnrc": "yarn", ".yarnrc.yml": "yarn", ".npmrc": "npm", ".npmignore": "npm",
	"deno.json": "deno", "deno.jsonc": "deno", "deno.lock": "deno", "bun.lockb": "bun",
	"bun.lock": "bun", "bunfig.toml": "bun", "turbo.json": "turborepo", "turbo.jsonc": "turborepo",
	".huskyrc": "husky", ".huskyrc.json": "husky", ".huskyrc.js": "husky",
	"husky.config.js": "husky", "ruff.toml": "ruff", ".ruff.toml": "ruff",

	// 代码与脚本
	".go": "go", ".rs": "rust", ".py": "python", ".pyi": "python", ".pyw": "python",
	".ts": "typescript", ".mts": "typescript", ".cts": "typescript", ".tsx": "react_ts",
	".js": "javascript", ".mjs": "javascript", ".cjs": "javascript", ".jsx": "react",
	".c": "c", ".h": "h", ".hpp": "h", ".hh": "h", ".hxx": "h", ".cpp": "cpp", ".cc": "cpp",
	".cxx": "cpp", ".m": "objective-c", ".mm": "objective-cpp", ".java": "java",
	".swift": "swift", ".kt": "kotlin", ".kts": "kotlin", ".cs": "csharp", ".fs": "fsharp",
	".fsx": "fsharp", ".fsi": "fsharp", ".fsproj": "fsharp", ".rb": "ruby", ".erb": "ruby",
	".gemspec": "ruby", ".rake": "ruby", ".php": "php", ".lua": "lua", ".dart": "dart",
	".scala": "scala", ".sc": "scala", ".hs": "haskell", ".lhs": "haskell", ".ex": "elixir",
	".exs": "elixir", ".eex": "elixir", ".heex": "elixir", ".leex": "elixir", ".erl": "erlang",
	".clj": "clojure", ".cljs": "clojure", ".cljc": "clojure", ".edn": "clojure",
	".jl": "julia", ".groovy": "groovy", ".gvy": "groovy", ".zig": "zig", ".zon": "zig",
	".pl": "perl", ".pm": "perl", ".pod": "perl", ".r": "r", ".rmd": "r", ".ml": "ocaml",
	".mli": "ocaml", ".nim": "nim", ".nimble": "nim", ".lisp": "lisp", ".lsp": "lisp",
	".coffee": "coffee", ".cson": "coffee", ".f": "fortran", ".f77": "fortran",
	".f90": "fortran", ".f95": "fortran", ".f03": "fortran", ".f08": "fortran", ".cu": "cuda",
	".cuh": "cuda", ".sol": "solidity", ".rkt": "racket", ".ss": "scheme", ".scm": "scheme",
	".pas": "pascal", ".sv": "verilog", ".svh": "verilog", ".vhd": "verilog",
	".vhdl": "verilog", ".elm": "elm", ".gleam": "gleam", ".nix": "nix", ".typ": "typst",
	".gd": "godot", ".ino": "arduino", ".applescript": "applescript", ".cr": "crystal",
	".ecr": "crystal", ".ada": "ada", ".adb": "ada", ".ads": "ada", ".cob": "cobol",
	".cbl": "cobol", ".d": "d", ".asm": "assembly", ".nasm": "assembly", ".s": "assembly",
	".glsl": "shader", ".vert": "shader", ".frag": "shader", ".tesc": "shader",
	".tese": "shader", ".geom": "shader", ".comp": "shader", ".hlsl": "shader",
	".wgsl": "shader", ".spv": "shader", ".shader": "shader", ".fx": "shader",
	".cginc": "shader", ".proto": "proto", ".vue": "vue", ".svelte": "svelte", ".astro": "astro",
	".ipynb": "jupyter", ".tf": "terraform", ".tfvars": "terraform", ".graphql": "graphql",
	".gql": "graphql", ".sql": "database", ".db": "database", ".sqlite": "database",
	".sqlite3": "database", ".sh": "console", ".zsh": "console", ".bash": "console",
	".fish": "console", ".bat": "console", ".cmd": "console", ".ps1": "powershell",
	".psm1": "powershell", ".mk": "makefile", ".cmake": "cmake", ".gradle": "gradle",
	".bzl": "bazel", ".bazel": "bazel", ".vim": "vim", ".vimrc": "vim", ".gvimrc": "vim",
	".exrc": "vim", ".viminfo": "vim", ".csproj": "visualstudio", ".sln": "visualstudio",
	".slnf": "visualstudio", ".slnx": "visualstudio", ".vcxproj": "visualstudio",
	".vb": "visualstudio", ".vbs": "visualstudio", ".razor": "razor", ".cshtml": "razor",
	".vbhtml": "razor", ".xaml": "xaml", ".wasm": "webassembly", ".wat": "webassembly",
	".hbs": "handlebars", ".mustache": "handlebars", ".handlebars": "handlebars",
	".hjs": "handlebars", ".ejs": "ejs", ".jinja": "jinja", ".jinja2": "jinja", ".j2": "jinja",
	".twig": "twig", ".pug": "pug", ".jade": "pug", ".haml": "haml", ".liquid": "liquid",
	".njk": "nunjucks", ".nunjucks": "nunjucks", ".mjml": "mjml",

	// 网页、标记、数据与配置
	".html": "html", ".htm": "html", ".xhtml": "html", ".css": "css", ".scss": "sass",
	".sass": "sass", ".less": "less", ".pcss": "postcss", ".sss": "postcss", ".md": "markdown",
	".markdown": "markdown", ".rst": "markdown", ".mdx": "mdx", ".tex": "tex", ".ltx": "tex",
	".cls": "tex", ".clo": "tex", ".sty": "tex", ".latex": "tex", ".aux": "tex", ".tikz": "tex",
	".bib": "bibliography", ".bbl": "bibliography", ".bcf": "bibliography",
	".blg": "bibliography", ".yaml": "yaml", ".yml": "yaml", ".json": "json",
	".jsonc": "json", ".json5": "json", ".jsonl": "json", ".ndjson": "json", ".toml": "toml",
	".xml": "xml", ".plist": "xml", ".xsd": "xml", ".xsl": "xml", ".xslt": "xml",
	".dtd": "xml", ".nuspec": "nuget", ".nupkg": "nuget", ".ini": "settings",
	".conf": "settings", ".cfg": "settings", ".config": "settings", ".properties": "settings",
	".nginx": "nginx", ".po": "i18n", ".pot": "i18n", ".mo": "i18n", ".xlf": "i18n",
	".lock": "lock", ".patch": "git", ".log": "log", ".http": "http", ".rest": "http",
	".drawio": "drawio", ".dio": "drawio", ".mmd": "mermaid", ".mermaid": "mermaid",
	".puml": "uml", ".plantuml": "uml", ".pu": "uml", ".iuml": "uml", ".wsd": "uml",
	".excalidraw": "excalidraw", ".todo": "todo", ".pem": "key", ".key": "key",
	".pub": "key", ".gpg": "key", ".asc": "key", ".crt": "certificate", ".cer": "certificate",
	".cert": "certificate",

	// 图片、设计与 3D
	".png": "image", ".jpg": "image", ".jpeg": "image", ".gif": "image", ".webp": "image",
	".ico": "image", ".bmp": "image", ".tif": "image", ".tiff": "image", ".avif": "image",
	".heic": "image", ".svg": "svg", ".psd": "adobe-photoshop", ".psb": "adobe-photoshop",
	".ai": "adobe-illustrator", ".fig": "figma", ".sketch": "sketch", ".blend": "blender",
	".blend1": "blender", ".stl": "3d", ".stp": "3d", ".step": "3d", ".obj": "3d",
	".fbx": "3d", ".dwg": "3d", ".dxf": "3d", ".3ds": "3d", ".dae": "3d", ".gltf": "3d",
	".glb": "3d", ".ply": "3d", ".usdz": "3d", ".3mf": "3d", ".skp": "3d",

	// 文档、表格、演示、电子书、邮件、字幕、音视频、字体与压缩包
	".pdf": "pdf", ".doc": "word", ".docx": "word", ".rtf": "word", ".odt": "word",
	".ppt": "powerpoint", ".pptx": "powerpoint", ".pptm": "powerpoint", ".odp": "powerpoint",
	".csv": "table", ".tsv": "table", ".xls": "table", ".xlsx": "table", ".xlsm": "table",
	".ods": "table", ".epub": "epub", ".eml": "email", ".emlx": "email", ".msg": "email",
	".ics": "email", ".mbox": "email", ".pst": "email", ".ost": "email", ".olm": "email",
	".srt": "subtitles", ".vtt": "subtitles", ".ass": "subtitles", ".ssa": "subtitles",
	".sbv": "subtitles", ".ttml": "subtitles", ".mp4": "video", ".m4v": "video",
	".mov": "video", ".avi": "video", ".mkv": "video", ".webm": "video", ".wmv": "video",
	".flv": "video", ".mpg": "video", ".mpeg": "video", ".mp3": "audio", ".wav": "audio",
	".flac": "audio", ".m4a": "audio", ".aac": "audio", ".aif": "audio", ".aiff": "audio",
	".ogg": "audio", ".ttf": "font", ".otf": "font", ".woff": "font", ".woff2": "font",
	".ttc": "font", ".eot": "font", ".zip": "zip", ".tar": "zip", ".gz": "zip", ".tgz": "zip",
	".bz2": "zip", ".xz": "zip", ".zst": "zip", ".7z": "zip", ".rar": "zip",

	// 二进制与系统
	".exe": "exe", ".msi": "exe", ".dll": "dll", ".so": "dll", ".dylib": "dll", ".bin": "hex",
	".dat": "hex", ".hex": "hex", ".iso": "disc", ".dmg": "disc", ".vmdk": "disc",
	".qcow2": "disc", ".jar": "jar", ".class": "javaclass", ".a": "lib", ".lib": "lib",
	".pyc": "python-misc", ".whl": "python-misc", ".egg": "python-misc", ".apk": "android",
	".dex": "android", ".smali": "android",
}

// fileIcons 是用到的文件类型图标，按图标名；启动时就全部解析，图标名写错在这时就暴露。
var fileIcons = func() map[string]*ui.SVG {
	icons := map[string]*ui.SVG{"document": icon("material/document")}
	for _, name := range fileIconNames {
		if icons[name] == nil {
			icons[name] = icon("material/" + name)
		}
	}
	return icons
}()

// fileIcon 返回路径对应的图标：目录是界面里通用的文件夹，文件按名称或扩展名找，
// 都不认识就是一页文档。
func fileIcon(p string) *ui.SVG {
	if strings.HasSuffix(p, "/") {
		return folderIcon
	}
	base := strings.ToLower(path.Base(p))
	name := cmp.Or(fileIconNames[base], fileIconNames[path.Ext(base)], "document")
	return fileIcons[name]
}

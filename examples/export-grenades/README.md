# 赛事 Demo 道具导出

从赛事 demo 目录中提取全部投掷物（烟 / 闪 / 雷 / 火 / 诱饵弹），导出为可标注的 CSV。

典型目录结构：赛事根目录下按场次分子目录，每场包含 2–5 个 `.dem` 文件。

```
tournament/
  liquid-vs-navi/
    m1-mirage.dem
    m2-inferno.dem
    m3-nuke.dem
  faze-vs-vitality/
    m1-anubis.dem
    m2-dust2.dem
```

## 环境要求

- [Go 1.24+](https://go.dev/dl/)
- Git
- 能访问 GitHub（用于 clone 和下载 Go 模块）

安装 Go 后确认版本：

```bash
go version
```

## 从 Git clone 到本地构建

```bash
git clone https://github.com/xu-mk/demoinfocs-golang.git
cd demoinfocs-golang
git checkout cursor/export-tournament-grenades-b03e
```

进入工具目录，下载依赖并编译：

```bash
cd examples/export-grenades
go mod download
go build -o export-grenades .
```

编译成功后，当前目录会生成可执行文件 `export-grenades`（Windows 上为 `export-grenades.exe`）。

不编译、直接运行也可以：

```bash
go run . -dir /path/to/tournament -out grenades.csv
```

之后若仓库有更新：

```bash
cd /path/to/demoinfocs-golang
git pull origin cursor/export-tournament-grenades-b03e
cd examples/export-grenades
go build -o export-grenades .
```

## 使用方法

导出整个赛事：

```bash
./export-grenades -dir /path/to/tournament -out grenades.csv
```

导出单个 demo：

```bash
./export-grenades -demo /path/to/demo.dem -out grenades.csv
```

不写 `-out` 时，CSV 输出到标准输出。进度和错误信息始终打印到 stderr。

### 参数

| 参数 | 说明 |
|-|-|
| `-dir` | 赛事 demo 根目录（递归查找全部 `.dem`） |
| `-demo` | 单个 demo 文件路径（与 `-dir` 二选一） |
| `-out` | 输出 CSV 路径；省略则写到 stdout |
| `-overwrite` | 覆盖已有 CSV，从头开始，而不是断点续跑 |

## 断点续跑

批量解析时，**每完成一个 demo 就会立刻写入并刷盘**。中途崩溃后，用同一条命令再跑一次即可：

```bash
./export-grenades -dir /path/to/tournament -out grenades.csv
```

已经导出的 demo 会被跳过（依据 CSV 中的「道具所属demo」列，以及同目录的 `grenades.csv.progress`）。某个 demo 损坏或解析失败时，会打印错误并继续后面的文件。

若要整份重来：

```bash
./export-grenades -dir /path/to/tournament -out grenades.csv -overwrite
```

## CSV 列说明

文件为 **UTF-8 带 BOM**，可用 Excel / WPS 直接打开，中文表头不会乱码。

| 列 | 说明 |
|-|-|
| 道具所属地图 | 地图名，如 `de_mirage` |
| 道具所属demo | 相对 `-dir` 的 demo 路径，如 `liquid-vs-navi/m1-mirage.dem` |
| 道具种类 | `烟` / `闪` / `雷` / `火` / `诱饵弹` |
| 道具投掷者 | 投掷者名字 |
| 起点X / 起点Y / 起点Z | 投掷瞬间投掷者坐标 |
| 准星角度X / 准星角度Y | 投掷瞬间准星角度（pitch / yaw） |
| 爆点X / 爆点Y / 爆点Z | 爆点坐标 |
| 道具分类 | 空列，留给标注 |
| setpos/setang | `setpos <x> <y> <z>; setang <pitch> <yaw>`，可直接复制到游戏控制台 |

火瓶和燃烧弹都记为 `火`。

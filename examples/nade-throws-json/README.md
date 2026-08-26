# nade-throws-json

按回合导出 CS2 demo 中的道具投掷信息为 JSON，可作为本地 CLI 或简单 HTTP 后端使用。

## 输出字段

每个道具包含：

| 字段 | 说明 |
|------|------|
| `type` | 道具种类 |
| `id` | 道具 UniqueID |
| `thrower` | 投掷者（name / steam_id_64 / team） |
| `thrower_position` | 投掷时投掷者位置 |
| `thrower_view_angles` | 投掷时准星角度（yaw / pitch） |
| `start` / `end` | 投掷物起落点 |
| `throw_method` | 投掷方式 |

### `throw_method`

| 值 | 含义 |
|----|------|
| `standing_still` | 站立不动投掷：投掷前短窗口内 xyz 均无明显位移 |
| `standing_jump` | 站立不动跳投：仅 z 有位移，且投掷时 `IsAirborne()` |
| `running_jump` | 跑跳投：xy 与 z 均有位移，且投掷时 `IsAirborne()` |
| `other` | 未落入以上三类（例如跑投不跳、落地滑步等） |

## CLI

```bash
go run . -demo /path/to/demo.dem -out throws.json
```

## HTTP 后端

```bash
go run . -listen :8080
```

```bash
curl -F demo=@/path/to/demo.dem http://localhost:8080/export -o throws.json
```

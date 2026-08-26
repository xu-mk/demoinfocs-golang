# nade-throws-json

按回合导出 CS2 demo 中的道具投掷信息为 JSON，可作为本地 CLI 或简单 HTTP 后端使用。

本版本**不做投掷方式分类**，只导出投掷瞬间状态字段。

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
| `ground_speed` | 投掷瞬间地速（XY，单位 u/s，近似 `cl_showpos` 的 `vel`） |
| `airborne` | 投掷瞬间是否悬空（`IsAirborne()`） |
| `tick` | 投掷时的 ingame tick |

> 说明：CS2 demo 通常不直接联网玩家速度，`ground_speed` 由相邻采样点的水平位移 / 时间估算，可能与客户端 `vel` 有少量偏差。

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

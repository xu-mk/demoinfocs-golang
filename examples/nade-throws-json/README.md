# nade-throws-json

按回合导出 CS2 demo 中的道具投掷信息为 JSON，可作为本地 CLI 或简单 HTTP 后端使用。

投掷方式由 4 部分拼接：**特殊移动 + 方向 + 移动程度 + 是否跳投 + 鼠标按键**。

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
| `throw_method` | 投掷方式中文标签 |
| `tick` | 投掷时的 ingame tick |

### `throw_method` 规则

1. **鼠标按键**（看投掷前最后一次 Attack / Attack2 按住过程）
   - 只按过 Attack：`左键`
   - 只按过 Attack2：`右键`
   - 两个都按过：`双键`
2. **特殊移动 / 方向**
   - `蹲`：投掷瞬间必须仍按住 Duck
   - `shift`：投掷瞬间必须仍按住 Speed，且地速不为 0
   - 方向 `w/a/s/d`：地速不为 0，且取投掷前最后一次方向键按住（含已松开的跑一步、或仍按住的跑投）
   - 地速约为 0：忽略静走，只标注是否蹲下；未蹲则为 `站`
3. **移动程度**（地速）
   - 静走（shift）时不判断
   - 约 245：`跑`
   - 约 130：`跑一步`
   - 约 30 且有方向键的跳投：不加 `跑` / `跑一步`（例如 `w跳左键投`）
4. **跳投**：投掷瞬间 `airborne == true` 则加 `跳`

示例：

- `蹲w跑一步跳左键投`
- `w跳左键投`
- `d跑跳左键投`
- `右键站投`

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

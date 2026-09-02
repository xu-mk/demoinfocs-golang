# Export grenade data for annotation

Walks a tournament demo directory and exports every thrown grenade to a CSV
file that can be used for annotation.

Typical layout: a tournament folder contains one directory per match, and each
match directory holds 2–5 `.dem` files.

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

## Running the example

Export a whole tournament:

```
go run . -dir /path/to/tournament -out grenades.csv
```

Export a single demo:

```
go run . -demo /path/to/demo.dem -out grenades.csv
```

Omit `-out` to write the CSV to stdout. Progress messages always go to stderr.

## CSV columns

| Column | Description |
|-|-|
| 地图 | Map name (e.g. `de_mirage`) |
| demo路径 | Absolute path of the demo file |
| 种类 | Grenade type: `烟` (smoke), `闪` (flash), `雷` (HE), `火` (molotov / incendiary), `诱饵弹` (decoy) |
| 投掷者 | Thrower name |
| 起点X / 起点Y / 起点Z | Throw / start coordinates |
| 爆点X / 爆点Y / 爆点Z | Detonation coordinates |
| 分类 | Empty, reserved for annotation |

Molotov and incendiary grenades are both exported as `火`.

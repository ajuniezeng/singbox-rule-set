This repository consumes `geoip.dat` and `geosite.dat` from the latest release of
`Loyalsoldier/v2ray-rules-dat` and converts them into sing-box `.srs` rule-set
artifacts.

Upstream project:
- Repository: <https://github.com/Loyalsoldier/v2ray-rules-dat>
- Release assets used here: `geoip.dat`, `geosite.dat`
- Upstream license: GNU General Public License v3.0

Changes in this repository:
- switched the upstream source from `v2fly/domain-list-community` release data to
  `Loyalsoldier/v2ray-rules-dat`
- added checksum-verified downloads for both `geoip.dat` and `geosite.dat`
- generate downstream sing-box rule-set artifacts from the upstream `.dat` files

License notice:
- this repository includes original GPL-licensed work and downstream
  modifications by AjunieZeng <contact@ajunie.com>
- original copyright notices and GPL terms must be preserved when
  redistributing this work or derivative works

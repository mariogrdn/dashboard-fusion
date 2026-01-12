# Dashboard Fusion

Dashboard Fusion is a tool for merging and updating Grafana dashboards by combining panels from different sources. 
It's designed to be used when working with dashboards that share common panels and groups.
It allows to update existing panels, while preserving the dashboard layout.

## Fusion Behavior

Fusion is performed by merging panels from multiple sources into a single dashboard.
- Dashboards are merged group first by matching the group name ( case sensitive ). 
- Inside a group, panels are matched by title and type, with the new panels appended to the bottom.
- New groups are appended to the bottom of the dashboard ( if the 'top' is not specified ).
Later, the dashboard can be manually reorganized to achieve the desired layout.

## Usage

```bash
dashboard-fusion
      --dash string      Location of base dashboard [required]
      --out string       Location of updated dashboard, defaults to stdout
      --panels strings   Location of panel(s) to be merged into base dashboard [required]
      --top bool         Append new groups/panels to the top of the destination dashboard instead of the default bottom
```

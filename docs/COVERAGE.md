# gotdx 能力覆盖矩阵

稳定公共接口按上游协议客户端分组，并使用固定路由映射到 `gotdx` 方法：普通主市场客户端统一使用 `/api/v1/stocks`，MAC 客户端统一使用 `/api/v1/mac`。不提供由用户传入方法名的通用执行接口。

公共参数仅保留业务语义字段：全市场接口的 `market`、带 `.SH` 或 `.SZ` 后缀的 `code`、批量 `symbols`、非 K 线分页 `offset/limit`、K 线 `start_date/end_date`、`date`、K 线 `period/adjust` 和多日分时 `days`。按证券接口由 `code` 后缀推导市场，不接受 `market`。协议内部的 `start`、`count`、数字 `category`、`times`、位图、排序、过滤和服务端偏移均由服务器管理，不对外暴露。

| API 分组 | gotdx 稳定能力 | 公共路由前缀 |
| --- | --- | --- |
| 主市场证券与行情 | `StockCount`、`StockList`、`StockQuotesDetail` | `/api/v1/stocks` |
| K 线与分时 | `StockKLine`、`GetIndexBars`、`StockTickChart`、`StockHistoryTickChart`、`StockChartSampling` | `/api/v1/stocks`，`code` 使用查询参数 |
| 成交与市场分析 | `StockTransaction`、`StockHistoryOrders`、`StockHistoryTransaction`、`StockIndexInfo`、`StockIndexMomentum`、`StockAuction`、`StockUnusual`、`StockVolumeProfile` | `/api/v1/stocks` |
| 主站信息 | `GetExchangeAnnouncement`、`GetAnnouncement` | `/api/v1/stocks` |
| F10 与公司资料 | `GetFinanceInfo`、`GetXDXRInfo`、`StockF10` | `/api/v1/stocks`，`code` 使用查询参数 |
| MAC 板块 | `MACBoardList`、`MACBoardMembers`、`MACBoardMembersQuotes`；列表和成分 SQLite 优先 | `/api/v1/mac/boards`，`board_symbol` 使用查询参数 |
| MAC 股票 | `MACSymbolQuotes`、`MACQuotesWithDate`、`MACTransactionsWithDate`、`MACAuction`、`MACTickCharts`、`MACSymbolInfo`、`MACCapitalFlow`、`MACMarketMonitor`、`MACSymbolBelongBoard`、`MACSymbolBars` | `/api/v1/mac` |

## 明确排除

- 商品语义：全部 `Goods*` 方法。
- 非沪深市场：北京、香港、美国以及全部扩展市场 `Ex*` 方法。
- 原始文件：`GetFileMeta`、`DownloadFullFile`、`Get*File`、`MACFileList`、`MACDownloadFullFile` 等仅供服务器内部使用。
- 协议运维与调试：服务器信息、心跳、K 线偏移、动态位图、排序和过滤接口不公开。
- ICFQS：全部 `ICFQS*`、`PostTQL` 和 `PostJSON` 方法。
- 主站实验协议：`MainTodo*`、`MainClient*` 及对应底层方法。
- 扩展实验协议：`ExExperiment*`、`ExMapping2562`、`ExListExtra`、`ExKLine2`、`ExQuotes2`。
- 主站旧版或兼容协议：`StockListOld`、`StockFeature452`、`StockQuotesEncrypt` 和 `*WithTrans`。

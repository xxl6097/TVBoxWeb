# 优化档案（已归档）

这里是 2026-11-08 ~ 11-11 期间「夜帧影视」三轮优化的原始分析稿与阶段报告。
**文件本身是历史记录，改动请以代码为准**；只有 `OPTIMIZATION_TASKS.md` 在 11-11 做过逐条回勾。

| 文件 | 作用 | 状态 |
|---|---|---|
| `OPTIMIZATION_ANALYSIS.md` | 最初的全面优化分析（问题清单 + 方案设想） | 历史稿，部分方案已被更好的实现替代 |
| `OPTIMIZATION_TASKS.md` | 12 个任务的执行清单 | **已按代码逐条回勾**，12 项里 11 项完成 |
| `OPTIMIZATION_PHASE1_REPORT.md` | 一期（直播搜索 / Seek 防抖 / Docker / 连播 / 字幕 / 倍速） | 已完成 |
| `OPTIMIZATION_PHASE2_REPORT.md` | 二期（并发测速 / API 缓存 / 站源健康度） | 已完成 |
| `OPTIMIZATION_PHASE3_REPORT.md` | 三期（收藏分组 / 健康检查 / 骨架屏） | 已完成 |

## 为什么归档

- 这些文档原本散在 workspace 根目录，不在版本控制里，容易和代码脱节
- 旧版 `OPTIMIZATION_TASKS.md` 有 76 个未勾选框，会让人误以为功能都没做（实际绝大多数早已落地）
- 现在回勾完成、随代码一起进 git，后续判断「做过没做过」直接看这里，不必再翻提交历史

## 后续优化记录去哪看

- 版本级变更说明：`README.md` 的「🆕 vX.Y.Z」章节
- 在线升级相关：`UPGRADE_GUIDE.md`
- 更细的取舍与踩坑：`.workbuddy/memory/` 下的工作日志

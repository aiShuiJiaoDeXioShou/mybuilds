# 011具体Go接入契约候选

root复核冻结后实施。010/011共同ApplicationBinding/PublishIntent/PublishGrant/PublishReceipt/PublishExpectation、Store方法与唯一RemoteOptions.Publish字段以[010 Go契约](../../010-google-play/contracts/go-api.md)为单一定义；本页只增加Apple真实消费者字段/规则。所有JSON snake_case，新增可选variant omitempty，私有材料json:"-"；旧无upload消息digest保持。019报告字段以正式验收契约串行同步。

## Apple节点具体方法

```go
type AppleOptions struct {
    BundleDir, DataDir, CredentialFile string // 只节点，同进程受限路径
    AppIdentifier, VersionName, DistributionTeamID string
    Number int64
    ArtifactPath, ArtifactSHA256 string
    ArtifactSize int64
}
type PreparedApple struct {
    workDir, credentialCopy, artifactCopy string
    options AppleOptions
    materialDigest, toolLockDigest string
    closed bool
}
func PrepareApple(ctx context.Context, in AppleOptions) (*PreparedApple, error)
func UploadApple(ctx context.Context, p *PreparedApple,
    grant protocol.PublishGrant, onStart func(process.StartInfo) error) (protocol.PublishReceipt, error)
func QueryApple(ctx context.Context, in AppleOptions,
    task protocol.PublishQueryTask) (protocol.PublishQueryResult, error)
func (p *PreparedApple) Close() error
```

位于internal/distribute；只有Google/Apple两个具体publisher，原process.Run，固定受控Ruby入口，非interface/registry/第二executor。Prepare只受限Go文件读取、实际codesign/profile/包核验与只读工具/Apple前提，耗时计原普通NS；DistributionTeamID来自原005真实签名profile/证书核验，不能用API issuer_id替代。Close只清自产0700/0600材料、有限独立系统清理预算，不撤回submission或执行remote abort。

Ruby/transport原stdout、stderr、HTTPbody只进受限有界诊断文件/读取器，不直接镜像到Run公开日志；固定入口只返回有界严格安全结果，Go核对已授action与digest，再由原logger输出固定状态摘要。材料值、JWT、私有路径注入扫描包含失败/verbose分支；不能依赖仅env引用路径的现有脱敏器隐藏p8正文。实际process.Run的writer错误/Authority回收及CleanupFailed仍保留。

每个UploadApple调用只接受一个具体已授权Apple Action，不允许Runner.run或SubmitForReview.submit!再派生其它未授权写请求。Agent同一个RemoteOptions.Publish闭包依固定六动作顺序Prepare→生成IntentID并journal fsync→Authorize→保存实际grant→UploadApple→本地receipt→Record；中间仅GET前提/processing，失败不重跑先前动作。onStart用于实际进程Started/当前action物理证据，pipeline同一个upload step仅第一次真实process.Start提交started一次；后续真实action start更新私有发布journal而非伪造重复步骤started。

## optional Apple字段（root唯一协议writer）

共同PublishAuthorization新增Apple *ApplePublishAuthorization；共同PublishGrant、PublishRemoteEvidence及QueryResult/安全DTO按实际需要引用Apple字段；不复制共同Ref/digest/artifact/report定义。

```go
type ApplePublishAuthorization struct {
    Action, PreviousIntentID string
    RequestSHA256 string
    AppStoreVersionID, BuildID, ReviewSubmissionID, ReviewItemID string
    SubmitForReview, AutomaticRelease bool
}
type AppleRemoteEvidence struct {
    AppID, BuildID, AppStoreVersionID, ReviewSubmissionID, ReviewItemID string
    ProcessingState, VersionState, ReviewState, ReleaseType string
    TransportID, RequestSHA256, ResponseSHA256 string
    UploadedAt *time.Time
    ActionConfirmed bool
}
```

Grant.Apple使用同一ApplePublishAuthorization，RequestSHA256在Authorize前冻结并由AuthorizationDigest绑定；实际回执的同名字段必须相等。REST动作摘要来自固定发送的规范JSON body；upload_binary摘要来自app/version/number/原IPA size与SHA256的固定描述，不含材料路径、JWT、密钥或URLquery。远端ID必须有界非控制字符串≤128bytes，状态只锁版本Apple枚举白名单；摘要64hex。TransportID仅可公开的关联ID，绝非token/JWT/sessioncookie。ReportIDs精确来自原seal，不能Agent自填pass。未适用字段omitempty；非法null/重复key/unknown字段拒绝。

PublishAuthorization共同VersionName/VersionCode分别对应IPA CFBundleShortVersionString/项目Number转CFBundleVersion；AppIdentifier必须与绑定bundle ID一致。Apple.Action下面精确值不接受任意method/path；首PreviousIntentID空，其后精确前动作已确认ID，同Ref/Index/chain。Submit/Automatic必须原Step值，无覆盖入口。

## 六个真实变更

| Action | 允许的唯一变更 | 必须前提/证据 |
|---|---|---|
| upload_binary | 受控deliver binary/实际transport的一次上传会话 | 唯一原签名IPA/摘要/报告seal，原app/version/number无冲突；回执实际transport关联，不仅exit0 |
| select_build | PATCH原appStoreVersion的build relationship | processing已VALID、精确原build ID，已有编辑版本；响应/GET关系完全匹配 |
| set_release_policy | PATCH原version.releaseType | 原explicit flags；false=MANUAL，true=AFTER_APPROVAL且submit=true；禁止auto date/phased等未授权变更 |
| create_review | POST reviewSubmissions | 已准备前提，无其它review进行；返回精确submission ID |
| add_review_item | POST reviewSubmissionItems | 仅原submission+原version，草稿不含其它items；返回item ID及精确关系 |
| submit_review | PATCH原reviewSubmission submitted=true | 原item/关系、READY_FOR_REVIEW、原明确submit；真实返回/后续GET已submitted |

所有GET不需各建动作意图；没有create_version/export_compliance/metadata/pricing/screenshot/reject/release_now动作。选择/策略已精确满足时仅作为下一动作的只读前提，不虚造已授权意图、回执或Started；下一动作仍绑定精确原build/version及明确选择。create_review不接管其它已有草稿，无本链来源不新建query草稿。

## Store共同入口的Apple规则

BindApplicationInput.Store=app_store，AppIdentifier bundle ID、NodeID原授权节点，CredentialRef仅完整节点env引用；实际节点只读doctor GET核验后verified。其它共同Store签名沿010，不新增workflow接口。

AuthorizePublish事务中重查当前独立actor/token、原lease/fence/控制锁/普通预算、admin+allowupload、original artifact/report seal和app唯一绑定，再持guard、插入具体动作unknown，一次返回grant；重复同动作只返原安全状态不新grant、不换号。末尾再次期限复核，不能先网络再存。

同upload步骤的应用guard在action间保留（绑定第一Intent根链，后动作PreviousIntentID），不能每次Record后解锁让另一build插入。ApplyEvent真实upload.finished先关闭本step授权slot，拒绝之后Authorize；只有已授动作完整集合均已确认或有无副作用失败证据、且真实停止才释放guard。终态PublishIntents再核对该完整集合；超预算时未授后动作保持未执行，不创建虚假意图。已经grant可能丢响应即unknown，不用缺Started解保护。原失败/取消原因与纳秒预算规则沿Run；停止未知禁止post，发布未知即使物理已停止仍保appguard但可释放普通node slot。

沿010 FindNodePublish/PublishLookup/NodePublishState，当前独立NodeActor先核对原Ref.NodeID，仅对本attempt/本step读取授权事实。请求前持久化的每个候选IntentID必须逐条精确查原条；slot未关闭时not_found不能证明未授权，slot已关闭且Authorized=false才可排除该候选。由这些实际单条读取组成完整terminal manifest，不增列表接口、不授grant、不延长旧lease，不与008停止receipt混用。

RecordPublish只精确IntentID/AuthDigest/Ref/receipt digest；旧fence拒写，同摘要幂等/冲突拒绝，不因副作用已经发生忽略当前权限。失权时实际receipt先留节点私有journal，后续query仅读原receipt/GET核对，不用旧Ref补写或重做。

Request/Claim/CompletePublishQuery沿010，仅当前node token/session领取原doctor/query任务；不借旧lease授写权。Apple Matches使用AppleRemoteEvidence的有限数组≤16，响应≤64KiB；GET30s/单次query总时限30s，ctx更早为准，不占buildslot。仅同版本存在但缺原transport/receipt关联不能确认unknown upload；submit须原submission ID+item/version/build关系及已提交状态，uploaded不能代替。充分关联才Store确认，否则safe evidence+unknown；空结果不证明未发送。

ConfirmPublishInput沿010，admin精确原intent/action/revision/digest及有限非机密依据；固定evidence code仅remote_receipt/remote_state/confirmed_not_sent/remote_rejected。相同决定幂等，冲突拒绝，无证据/仅pid或StopKnown拒绝，人工confirm不授第二动作。

## 安全结果与预算

状态枚举共同uploaded/processing/submitted/published/failed/unknown；内部action confirmed与远端生命周期分开。unknown与failed都不得猜published；uploaded已知而submit未授的超时保该真实事实，build仍按timeout失败。公布App Review REJECTED只更新已提交的外部结果，不抹上传/提交。

固定safe errors：apple_tools_missing、apple_tools_unverified、apple_credentials_invalid、apple_app_invalid、apple_artifact_invalid、apple_signature_invalid、apple_version_conflict、apple_reports_invalid、apple_prerequisite_missing、apple_processing_timeout、apple_authority_lost、apple_budget_exhausted、apple_unknown、apple_cleanup_failed。不回显原工具输出/OS路径/远端body。原NS不能从ms重算，所有准备/GET/授意图/实际写/receipt确认耗时由同Run预算消费者累计。

## 2026-10实际只读下一动作消费者

```go
func (p *PreparedApple) NextAction(ctx context.Context, previous []protocol.PublishReceipt,
    submit, automatic bool) (*protocol.ApplePublishAuthorization, error)
```

第一次返回upload_binary；已确认上传且submit=false返回nil。其后仅GET原app/Number/version与本链精确submission/item关系，每次返回一个尚未执行动作及实际IDs/RequestSHA256；原选择/策略已吻合仅只读跳过，不伪造receipt。Agent补PreviousIntentID再申请单次授权。processing等待受原ctx与30s共同约束，失败保已确认上传事实；缺可编辑既有版本或存在外部待审记录拒绝，不创建版本或接管外部审核。ReviewSubmission响应按官方state/submittedDate核对；ReviewSubmissionItem无reviewSubmission关系，核对原submission/items精确成员及appStoreVersion。

上传仅使用fastlane2.240.1的AltoolTransporterExecutor.prepare/build_upload_command；不调用其使用PTY且无限行缓存的execute。Go在实际核对完整argv、临时p8字节与原AppID后，由唯一process.Run直接执行/usr/bin/xcrun，真实OnStart、有限私有输出与取消/清理归原scope。唯一相关TransportID与确认语句必须来自成功会话实际有限字节，ResponseSHA256绑定这些字节；任何缺失/停止未知仍unknown。没有真实Apple材料时，不声称此关联格式和上传后处理门已远端验收。

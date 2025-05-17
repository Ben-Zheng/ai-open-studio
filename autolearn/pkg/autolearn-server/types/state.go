package types

type AutoLearnState int8

const (
	// 对外展示、外部可请求状态
	AutoLearnStateInitial       AutoLearnState = iota // 0
	AutoLearnStateToBeLearned                         // 1
	AutoLearnStateLearning                            // 2
	AutoLearnStateCompleted                           // 3
	AutoLearnStateStopped                             // 4
	AutoLearnStateInitialFailed                       // 5 deprecated
	AutoLearnStateException                           // 6 deprecated

	AutoLearnStateRecommendFailed          // 7
	AutoLearnStateRecommendSucceeded       // 8
	AutoLearnStateDatasetChecking          // 9
	AutoLearnStateDatasetCheckFailed       // 10
	AutoLearnStateDatasetCheckSucceeded    // 11
	AutoLearnStateSpeeding                 // 12
	AutoLearnStateSpeedFailed              // 13
	AutoLearnStateSpeedSucceeded           // 14
	AutoLearnStateMasterPodCreateFailed    // 15
	AutoLearnStateMasterPodCreateSucceeded // 16
	AutoLearnStatePodsCreateFailed         // 17  deprecated
	AutoLearnStateOptimizing               // 18
	AutoLearnStateOptimizeFailed           // 19
	AutoLearnStateOptimizeSucceeded        // 20
	DisplayStateScheduling                 // 21
	DisplayStateScheduleSuccess            // 22
	DisplayStateScheduleFailed             // 23

	// 涉及上面状态的添加，同步修改此处数字，保持状态值不变
	AutoLearnStateMasterPodCreating  = iota + 26 // 50
	AutoLearnStateMasterPodRunning               // 51
	AutoLearnStateMasterPodRunFailed             // 52
	AutoLearnStateWorkerAJobCreated              // 53
	AutoLearnStateWorkerAJobFailed               // 54
	AutoLearnStateRecommending                   // 55
	AutoLearnStateLearnFailed                    // 56
)

var AutoLearnStateFailedMap = map[AutoLearnState]bool{
	AutoLearnStateInitialFailed:         true,
	AutoLearnStateException:             true,
	AutoLearnStateRecommendFailed:       true,
	AutoLearnStateDatasetCheckFailed:    true,
	AutoLearnStateSpeedFailed:           true,
	AutoLearnStateMasterPodCreateFailed: true,
	AutoLearnStatePodsCreateFailed:      true,
	AutoLearnStateOptimizeFailed:        true,
}

var FinalState = []AutoLearnState{
	AutoLearnStateOptimizeFailed,
	AutoLearnStateOptimizeSucceeded,
	AutoLearnStateCompleted,
	AutoLearnStateStopped,
	AutoLearnStateInitialFailed,
	AutoLearnStateException,
	AutoLearnStateRecommendFailed,
	AutoLearnStateDatasetCheckFailed,
	AutoLearnStateSpeedFailed,
	AutoLearnStateMasterPodCreateFailed,
	AutoLearnStatePodsCreateFailed,
	AutoLearnStateWorkerAJobFailed,
	AutoLearnStateLearnFailed,
	AutoLearnStateMasterPodRunFailed,
}

var NotFinalState = []AutoLearnState{
	AutoLearnStateInitial,
	AutoLearnStateToBeLearned,
	AutoLearnStateLearning,
	AutoLearnStateRecommendSucceeded,
	AutoLearnStateDatasetChecking,
	AutoLearnStateDatasetCheckSucceeded,
	AutoLearnStateOptimizing,
	AutoLearnStateOptimizeSucceeded,
	AutoLearnStateSpeeding,
	AutoLearnStateSpeedSucceeded,
	AutoLearnStateMasterPodCreateSucceeded,
	AutoLearnStateMasterPodCreating,
	AutoLearnStateMasterPodRunning,
	AutoLearnStateWorkerAJobCreated,
	AutoLearnStateRecommending,
}

var LingeringTime = map[AutoLearnState]int{
	AutoLearnStateOptimizeSucceeded:        1,
	AutoLearnStateInitial:                  5,
	AutoLearnStateDatasetChecking:          20,
	AutoLearnStateDatasetCheckSucceeded:    1,
	AutoLearnStateSpeeding:                 60,
	AutoLearnStateSpeedSucceeded:           1,
	AutoLearnStateRecommending:             2,
	AutoLearnStateRecommendSucceeded:       1,
	AutoLearnStateMasterPodCreating:        20,
	AutoLearnStateMasterPodCreateSucceeded: 20,
	AutoLearnStateMasterPodRunning:         6 * 60, // 6 个小时给予数据校验
}

type StateEvents int8
type StateEventType int8

const (
	StateEventTypeInit StateEventType = iota
	StateEventTypeDatasetCheck
	StateEventTypeSpeed
	StateEventTypeRecommend
	StateEventTypeMasterPodCreate
	StateEventTypeAdvancedMasterPodCreate
	StateEventTypeMasterPodRun
	StateEventTypeWorkerAJobCreate
	StateEventTypeSchedule
	StateEventTypeLearn
	StateEventTypeStop
)

type StateEvent struct {
	T StateEventType
	A EventAction
}

type EventAction int8

const (
	EventActionDoing = iota
	EventActionSuccess
	EventActionFailed
	EventActionDoingTimeout
	EventActionStepinTimeout
)

var EventMap = map[StateEventType]map[EventAction][]AutoLearnState{
	StateEventTypeInit: {
		EventActionDoing: {AutoLearnStateOptimizeSucceeded, AutoLearnStateInitial},
	},
	StateEventTypeDatasetCheck: {
		EventActionStepinTimeout: {AutoLearnStateInitial, AutoLearnStateDatasetCheckFailed},
		EventActionDoing:         {AutoLearnStateInitial, AutoLearnStateDatasetChecking},
		EventActionSuccess:       {AutoLearnStateDatasetChecking, AutoLearnStateDatasetCheckSucceeded},
		EventActionFailed:        {AutoLearnStateDatasetChecking, AutoLearnStateDatasetCheckFailed},
		EventActionDoingTimeout:  {AutoLearnStateDatasetChecking, AutoLearnStateDatasetCheckFailed},
	},
	StateEventTypeSpeed: {
		EventActionStepinTimeout: {AutoLearnStateDatasetCheckSucceeded, AutoLearnStateSpeedFailed},
		EventActionDoing:         {AutoLearnStateDatasetCheckSucceeded, AutoLearnStateSpeeding},
		EventActionSuccess:       {AutoLearnStateSpeeding, AutoLearnStateSpeedSucceeded},
		EventActionFailed:        {AutoLearnStateSpeeding, AutoLearnStateSpeedFailed},
		EventActionDoingTimeout:  {AutoLearnStateSpeeding, AutoLearnStateSpeedFailed},
	},
	StateEventTypeRecommend: {
		EventActionStepinTimeout: {AutoLearnStateSpeedSucceeded, AutoLearnStateRecommendFailed},
		EventActionDoing:         {AutoLearnStateSpeedSucceeded, AutoLearnStateRecommending},
		EventActionSuccess:       {AutoLearnStateRecommending, AutoLearnStateRecommendSucceeded},
		EventActionFailed:        {AutoLearnStateRecommending, AutoLearnStateRecommendFailed},
		EventActionDoingTimeout:  {AutoLearnStateRecommending, AutoLearnStateRecommendFailed},
	},
	StateEventTypeMasterPodCreate: {
		EventActionStepinTimeout: {AutoLearnStateRecommendSucceeded, AutoLearnStateMasterPodCreateFailed},
		EventActionDoing:         {AutoLearnStateRecommendSucceeded, AutoLearnStateMasterPodCreating},
		EventActionSuccess:       {AutoLearnStateMasterPodCreating, AutoLearnStateMasterPodCreateSucceeded},
		EventActionFailed:        {AutoLearnStateMasterPodCreating, AutoLearnStateMasterPodCreateFailed},
		EventActionDoingTimeout:  {AutoLearnStateMasterPodCreating, AutoLearnStateMasterPodCreateFailed},
	},
	StateEventTypeAdvancedMasterPodCreate: {
		EventActionStepinTimeout: {AutoLearnStateSpeedSucceeded, AutoLearnStateMasterPodCreateFailed},
		EventActionDoing:         {AutoLearnStateSpeedSucceeded, AutoLearnStateMasterPodCreating},
		EventActionSuccess:       {AutoLearnStateMasterPodCreating, AutoLearnStateMasterPodCreateSucceeded},
		EventActionFailed:        {AutoLearnStateMasterPodCreating, AutoLearnStateMasterPodCreateFailed},
		EventActionDoingTimeout:  {AutoLearnStateMasterPodCreating, AutoLearnStateMasterPodCreateFailed},
	},
	StateEventTypeMasterPodRun: {
		EventActionStepinTimeout: {AutoLearnStateMasterPodCreateSucceeded, AutoLearnStateMasterPodRunFailed},
		EventActionDoing:         {AutoLearnStateMasterPodCreateSucceeded, AutoLearnStateMasterPodRunning},
	},
	StateEventTypeWorkerAJobCreate: {
		EventActionStepinTimeout: {AutoLearnStateMasterPodRunning, AutoLearnStateWorkerAJobFailed},
		EventActionDoing:         {AutoLearnStateMasterPodRunning, AutoLearnStateWorkerAJobCreated},
	},
	StateEventTypeLearn: {
		EventActionStepinTimeout: {AutoLearnStateWorkerAJobCreated, AutoLearnStateLearnFailed},
		EventActionDoing:         {AutoLearnStateWorkerAJobCreated, AutoLearnStateLearning},
		EventActionSuccess:       {AutoLearnStateLearning, AutoLearnStateCompleted},
		EventActionFailed:        {AutoLearnStateLearning, AutoLearnStateLearnFailed},
		EventActionDoingTimeout:  {AutoLearnStateLearning, AutoLearnStateLearnFailed},
	},
	StateEventTypeStop: {
		EventActionSuccess: {-1, AutoLearnStateStopped},
	},
}

func TimeoutFailedStateEvent(state AutoLearnState, advance bool) (*StateEvent, StateEventCode) {
	var T StateEventType
	var A EventAction
	switch state {
	case AutoLearnStateInitial:
		T, A = StateEventTypeDatasetCheck, EventActionStepinTimeout
	case AutoLearnStateDatasetChecking:
		T, A = StateEventTypeDatasetCheck, EventActionDoingTimeout
	case AutoLearnStateDatasetCheckSucceeded:
		T, A = StateEventTypeSpeed, EventActionStepinTimeout
	case AutoLearnStateSpeeding:
		T, A = StateEventTypeSpeed, EventActionDoingTimeout
	case AutoLearnStateSpeedSucceeded:
		T, A = StateEventTypeRecommend, EventActionStepinTimeout
		if advance {
			T, A = StateEventTypeAdvancedMasterPodCreate, EventActionStepinTimeout
		}
	case AutoLearnStateRecommending:
		T, A = StateEventTypeRecommend, EventActionDoingTimeout
	case AutoLearnStateRecommendSucceeded:
		T, A = StateEventTypeMasterPodCreate, EventActionStepinTimeout
	case AutoLearnStateMasterPodCreating:
		T, A = StateEventTypeMasterPodCreate, EventActionDoingTimeout
	case AutoLearnStateMasterPodCreateSucceeded:
		T, A = StateEventTypeMasterPodRun, EventActionStepinTimeout
	case AutoLearnStateMasterPodRunning:
		T, A = StateEventTypeWorkerAJobCreate, EventActionStepinTimeout
	case AutoLearnStateLearning:
		T, A = StateEventTypeLearn, EventActionDoingTimeout
	}

	return &StateEvent{T: T, A: A}, StateEventCodeWatcherTimeout
}

type StateEventCode int

const (
	StateEventCodeNothing StateEventCode = iota
	StateEventCodeCustom
	StateEventCodeWatcherTimeout
	StateEventCodeDatasetCheckFailed
	StateEventCodeDatasetInvalidFormat
	StateEventCodeContinuedLearnClassCheckFailed
	StateEventCodeContinuedLearnGenerateClassFailed
	StateEventCodeSpeedFailed
	StateEventCodeRecommendFailed
	StateEventCodeRecommendFailedByInvalidSDS
	StateEventCodeRecommendFailedByNotFulfill
	StateEventCodeRecommendFailedByNotFound
	StateEventCodeRecommendFailedByTimeout
	StateEventCodeCreateMasterPodFailed
	StateEventCodeScheduleTimeout
	StateEventCodeLearningFailed
	StateEventCodeLearningFailed101
	StateEventCodeLearningFailed201
	StateEventCodeLearningFailed202
	StateEventCodeLearningFailed301
	StateEventCodeLearningFailed302
	StateEventCodeLearningFailed303
	StateEventCodeLearningFailed307
	StateEventCodeLearningFailed401
	StateEventCodeLearningTimeout
	StateEventCodeLearningMasterPodKilled
	StateEventCodeGPUNotMatched
)

var EventCodeMessage = map[StateEventCode]string{
	StateEventCodeWatcherTimeout:                    "状态停留时长超限被终止，请重建或咨询客服",
	StateEventCodeDatasetCheckFailed:                "数据文件不存在或格式不符合要求, 请检查数据格式重建实验",
	StateEventCodeDatasetInvalidFormat:              "数据格式不符合要求，请检查数据集：分类任务需 classes 字段存在且不为空，检测和关键点任务需boxes字段存在，回归任务需包含 regressors 字段且不为空，分割任务需包含 segmentations 字段且不为空，OCR 任务需包含 texts 字段且不为空",
	StateEventCodeContinuedLearnClassCheckFailed:    "继续学习数据集属性类别和模型不匹配，请校验继续学习的数据集类型后重新创建",
	StateEventCodeContinuedLearnGenerateClassFailed: "学习任务属性更新失败，请重建实验",
	StateEventCodeSpeedFailed:                       "数据集加速失败，请重建实验",
	StateEventCodeRecommendFailed:                   "无法匹配到合适的算法，可能是应用类型与数据不匹配或者训验集张数不满足4张，请检查数据集",
	StateEventCodeRecommendFailedByInvalidSDS:       "无效的数据集导致无法推荐算法，请检查数据集",
	StateEventCodeRecommendFailedByNotFulfill:       "推荐算法个数不满足配置要求, 获取推荐算法失败，请修改设置的算法个数",
	StateEventCodeRecommendFailedByNotFound:         "没有可推荐的算法，请重新选择应用类型/数据集",
	StateEventCodeRecommendFailedByTimeout:          "算法推荐超时，可能由于数据集过大，请修改数据集或重建实验",
	StateEventCodeCreateMasterPodFailed:             "任务初始化失败，可能是配额或资源不足，请申请资源后重建",
	StateEventCodeScheduleTimeout:                   "调度超时，可能当前资源不足，请查看资源是否充足并重建或咨询客服",
	StateEventCodeLearningFailed:                    "训练出现未知错误，可能是超参设置错误或显存oom或数据集存在宽高为负等，具体请查询日志或重建实验",
	StateEventCodeLearningFailed101:                 "任务初始化失败, 请检查数据集是否存在非法数据，例如框的宽高存在负数等",
	StateEventCodeLearningFailed201:                 "找不到训练成功的模型，可能是训练时间不够，请增加训练时间",
	StateEventCodeLearningFailed202:                 "评测超时，请尝试减少测试图片数目到 15k 以内",
	StateEventCodeLearningFailed301:                 "运行内存不足，请重建；若两次报相同的错误，请查看日志或咨询客服",
	StateEventCodeLearningFailed302:                 "GPU报错，请重建；若两次报相同的错误，可能是超参设置不合适导致OOM，请修改超参",
	StateEventCodeLearningFailed303:                 "训练卡死，请重建；若两次报相同的错误，请查询日志后咨询客服",
	StateEventCodeLearningFailed307:                 "显存OOM，建议调整相关超参数来减少显存占用",
	StateEventCodeLearningFailed401:                 "无数据集权限，请确认数据集权限",
	StateEventCodeLearningTimeout:                   "训练超过预设时长未能自动结束，若未得到满意的模型，请请重建或咨询客服",
	StateEventCodeLearningMasterPodKilled:           "训练主任务节点被意外终止，请重建或咨询客服",
	StateEventCodeGPUNotMatched:                     "没有匹配到满足算法的 GPU, 请检查 GPU 配置或咨询客服",
}

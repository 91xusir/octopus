package op

import (
	"context"
	"fmt"
	"hash/fnv"
	"slices"
	"sort"
	"strings"

	"github.com/91xusir/octopus/internal/db"
	"github.com/91xusir/octopus/internal/model"
	"github.com/91xusir/octopus/internal/utils/cache"
	"gorm.io/gorm"
)

var (
	groupCache     = cache.New[int, model.Group](16) // 按主键保存完整分组配置。
	groupNameIndex = cache.New[string, int](16)      // 客户端模型名对应的分组主键。
)

// GroupList 返回缓存中的全部分组, 成员已补齐界面展示所需的名称与可用性, 按名称定序。
// 不含实时路由状态: 路由状态由 Relay 持有, 而 Relay 依赖本包, 故由处理器在返回前补齐。
// 定序是为 API Key 面板的模型选择器: 那里没有排序开关, 而缓存遍历顺序随机;
// 分组页自带升降序开关, 会按开关重排, 不依赖此顺序。
func GroupList() []model.Group {
	groups := make([]model.Group, 0, groupCache.Len())
	for _, group := range groupCache.GetAll() {
		groups = append(groups, groupSnapshot(group))
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	return groups
}

// GroupGet 返回指定分组的读取副本, 成员已补齐界面展示所需的名称与可用性。
// 不含实时路由状态: 与 GroupList 同理, 由处理器在返回前补齐。
func GroupGet(id int) (model.Group, error) {
	group, ok := groupCache.Get(id)
	if !ok {
		return model.Group{}, fmt.Errorf("group not found")
	}
	return groupSnapshot(group), nil
}

// GroupListModel 返回缓存中的全部分组模型名与全部直连模型 ID, 按名称定序并去重。
// 两个消费方都不提供排序开关: /v1/models 由第三方客户端直接展示, API Key 面板按返回顺序列出可用模型,
// 而缓存遍历顺序随机, 故顺序须由此处定稿。直连 ID 与显式分组同名时分组优先, 去重后只保留一份。
func GroupListModel() []string {
	models := make([]string, 0, groupCache.Len())
	for _, group := range groupCache.GetAll() {
		models = append(models, group.Name)
	}
	models = append(models, DirectModelNames()...)
	sort.Strings(models)
	return slices.Compact(models)
}

// GroupGetByName 返回客户端模型名称对应的分组配置, 供转发选路使用。
// 渠道或凭据被禁用及授权两侧缺失的成员不参与选路, 重新可用后会在下一轮读取时自动恢复。
// 显式分组优先: 名称命中分组索引即按分组转发, 未命中再按直连 ID 解析渠道模型。
func GroupGetByName(name string) (model.Group, error) {
	groupID, ok := groupNameIndex.Get(name)
	if !ok {
		group, ok := directModelGroup(name)
		if !ok {
			return model.Group{}, fmt.Errorf("group not found")
		}
		return group, nil
	}
	group, ok := groupCache.Get(groupID)
	if !ok {
		return model.Group{}, fmt.Errorf("group not found")
	}
	group = groupSnapshot(group)
	group.Items = slices.DeleteFunc(group.Items, func(item model.GroupItem) bool { return !item.Available })
	return group, nil
}

// GroupCreate 创建分组及其成员并刷新缓存, 返回创建后的分组。
// 成员的提交顺序即优先级顺序。
func GroupCreate(req *model.GroupCreateRequest, ctx context.Context) (*model.Group, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("group name is required")
	}
	group := model.Group{
		Name:        name,
		Mode:        req.Mode,
		RelayConfig: req.RelayConfig,
		Items:       make([]model.GroupItem, len(req.Items)),
	}
	if group.Mode == "" {
		group.Mode = model.GroupModeManual
	}
	model.NormalizeGroupRelayConfig(&group.RelayConfig)
	for i, item := range req.Items {
		group.Items[i] = model.GroupItem{ChannelGrantID: item.ChannelGrantID, Priority: i + 1}
	}
	if err := db.GetDB().WithContext(ctx).Create(&group).Error; err != nil {
		return nil, err
	}
	groupCache.Set(group.ID, group)
	groupNameIndex.Set(group.Name, group.ID)
	snapshot := groupSnapshot(group)
	return &snapshot, nil
}

// GroupUpdate 更新分组配置, 成员和当前成员，并返回刷新后的分组。
func GroupUpdate(id int, req *model.GroupUpdateRequest, ctx context.Context) (*model.Group, error) {
	oldGroup, ok := groupCache.Get(id)
	if !ok {
		return nil, fmt.Errorf("group not found")
	}
	oldName := oldGroup.Name

	var selectFields []string
	updates := model.Group{ID: id}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, fmt.Errorf("group name is required")
		}
		selectFields = append(selectFields, "name")
		updates.Name = name
	}
	if req.Mode != nil {
		selectFields = append(selectFields, "mode")
		updates.Mode = *req.Mode
	}
	if req.RelayConfig != nil {
		config := *req.RelayConfig
		model.NormalizeGroupRelayConfig(&config)
		selectFields = append(selectFields, "relay_config")
		updates.RelayConfig = config
	}

	var group model.Group
	err := db.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(selectFields) > 0 {
			if err := tx.Model(&model.Group{}).Where("id = ?", id).Select(selectFields).Updates(&updates).Error; err != nil {
				return fmt.Errorf("failed to update group: %w", err)
			}
		}
		if req.Items != nil {
			if err := syncGroupItems(tx, id, *req.Items); err != nil {
				return err
			}
		}
		if err := tx.Preload("Items").First(&group, id).Error; err != nil {
			return fmt.Errorf("failed to load updated group: %w", err)
		}
		// 当前成员在成员集合定稿后才写入: syncGroupItems 会清空指向已删除成员的当前成员,
		// 先写会被它覆盖; 归属校验同样只对最终集合成立。
		if req.ActiveItemID != nil {
			if *req.ActiveItemID != 0 && !slices.ContainsFunc(group.Items, func(item model.GroupItem) bool { return item.ID == *req.ActiveItemID }) {
				return fmt.Errorf("group item not found")
			}
			if err := tx.Model(&model.Group{}).Where("id = ?", id).Update("active_item_id", *req.ActiveItemID).Error; err != nil {
				return fmt.Errorf("failed to update active item: %w", err)
			}
			group.ActiveItemID = *req.ActiveItemID
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sortGroupItems(group.Items)
	groupCache.Set(group.ID, group)
	groupNameIndex.Set(group.Name, group.ID)
	if oldName != group.Name {
		groupNameIndex.Del(oldName)
	}
	snapshot := groupSnapshot(group)
	return &snapshot, nil
}

// syncGroupItems 按提交的成员集合新增, 重排与删除分组成员。
// 成员在分组内按渠道授权唯一, 该授权作为匹配依据, 由此已有成员保留其主键:
// 主键被分组的当前成员和 Relay 的路由状态引用, 换主键会让人工选择与冷却记录失效。
// 优先级一律按提交顺序重写, 前端只需提交当前排列, 无需自行算出哪些成员的顺序发生了变化。
func syncGroupItems(tx *gorm.DB, groupID int, requested []model.GroupItemInput) error {
	var existing []model.GroupItem
	if err := tx.Where("group_id = ?", groupID).Find(&existing).Error; err != nil {
		return fmt.Errorf("failed to load group items: %w", err)
	}
	existingByGrant := make(map[int]model.GroupItem, len(existing))
	for _, item := range existing {
		existingByGrant[item.ChannelGrantID] = item
	}

	for priority, requestedItem := range requested {
		current, ok := existingByGrant[requestedItem.ChannelGrantID]
		if !ok {
			newItem := model.GroupItem{GroupID: groupID, ChannelGrantID: requestedItem.ChannelGrantID, Priority: priority + 1}
			if err := tx.Create(&newItem).Error; err != nil {
				return fmt.Errorf("failed to create group item: %w", err)
			}
			continue
		}
		if current.Priority != priority+1 {
			if err := tx.Model(&model.GroupItem{}).Where("id = ?", current.ID).
				Update("priority", priority+1).Error; err != nil {
				return fmt.Errorf("failed to update group item: %w", err)
			}
		}
		delete(existingByGrant, requestedItem.ChannelGrantID)
	}

	deletedIDs := make([]int, 0, len(existingByGrant))
	for _, item := range existingByGrant {
		deletedIDs = append(deletedIDs, item.ID)
	}
	if len(deletedIDs) == 0 {
		return nil
	}
	// 被删掉的成员可能正是当前人工指定的成员, 需一并清空, 否则分组会指向一个已不存在的成员。
	if err := tx.Model(&model.Group{}).
		Where("id = ? AND active_item_id IN ?", groupID, deletedIDs).
		Update("active_item_id", 0).Error; err != nil {
		return fmt.Errorf("failed to clear active item: %w", err)
	}
	if err := tx.Delete(&model.GroupItem{}, deletedIDs).Error; err != nil {
		return fmt.Errorf("failed to delete group items: %w", err)
	}
	return nil
}

// GroupDel 删除分组及其成员，成员删除不会影响被其他分组引用的渠道授权。
func GroupDel(id int, ctx context.Context) error {
	group, ok := groupCache.Get(id)
	if !ok {
		return fmt.Errorf("group not found")
	}
	if err := db.GetDB().WithContext(ctx).Delete(&model.Group{}, id).Error; err != nil {
		return fmt.Errorf("failed to delete group: %w", err)
	}
	groupCache.Del(id)
	groupNameIndex.Del(group.Name)
	return nil
}

// groupRefreshCache 从数据库刷新完整分组缓存和名称索引。
// 缓存只存库内行, 成员的名称与可用性在读取时由 groupSnapshot 现算: 它们随渠道与凭据变化,
// 存进缓存就得在每次渠道改动后跟着刷新一遍。
func groupRefreshCache(ctx context.Context) error {
	groups := []model.Group{}
	if err := db.GetDB().WithContext(ctx).
		Preload("Items").
		Find(&groups).Error; err != nil {
		return err
	}
	groupCache.Clear()
	groupNameIndex.Clear()
	for _, group := range groups {
		sortGroupItems(group.Items)
		groupCache.Set(group.ID, group)
		groupNameIndex.Set(group.Name, group.ID)
	}
	return nil
}

// sortGroupItems 按优先级和主键生成稳定的成员顺序。
func sortGroupItems(items []model.GroupItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority < items[j].Priority
		}
		return items[i].ID < items[j].ID
	})
}

// groupSnapshot 为成员补齐授权两侧的名称, 所属渠道与可用性。
// 可用性在此一次定稿: 渠道与凭据均启用且模型, 凭据均存在时可转发, 否则仍列出该成员但标记不可用,
// 由此界面无需再按渠道列表回查, 也不会出现前后端各判一套的分歧。
func groupSnapshot(group model.Group) model.Group {
	// 成员恒为数组: 读取侧承诺该字段不为 null, 空分组也要给出空数组。
	group.Items = append(make([]model.GroupItem, 0, len(group.Items)), group.Items...)
	for i := range group.Items {
		grant, ok := channelGrantCache.Get(group.Items[i].ChannelGrantID)
		if !ok {
			continue
		}
		group.Items[i].Protocols = grant.Protocols
		channelModel, modelOK := channelModelCache.Get(grant.ChannelModelID)
		channelKey, keyOK := channelKeyCache.Get(grant.ChannelKeyID)
		if !modelOK || !keyOK {
			continue
		}
		group.Items[i].ChannelID = channelModel.ChannelID
		group.Items[i].ModelName = channelModel.Name
		group.Items[i].KeyName = channelKey.Name
		channel, channelOK := channelCache.Get(channelModel.ChannelID)
		if !channelOK {
			continue
		}
		group.Items[i].ChannelName = channel.Name
		group.Items[i].Available = channel.Enabled && channelKey.Enabled
	}
	return group
}

// 直连模型 ID 让每个渠道模型无需建分组即可直接调用: 客户端以 前缀:模型名 作为模型名请求,
// 前缀取渠道设置的模型前缀, 留空回退渠道名。多个渠道共用同一前缀时, 同名模型的直连 ID
// 聚合全部渠道互为故障转移; 需要更细的优先级控制或手动指定成员时, 再显式创建分组。
const directModelSeparator = ":"

// directGroupIDBase 让直连分组的进程内路由状态与显式分组共用一个键空间又不与之冲突:
// 显式分组主键是自增小整数, 直连分组取基准值加直连 ID 的哈希。
const directGroupIDBase = 1 << 40

// directModelGroup 把直连模型 ID 解析为转发用的临时分组, 未命中任何渠道模型时返回假。
// 从左到右尝试每个冒号切分点: 前缀是渠道上设置的值, 名称含多个冒号时取最左侧能命中渠道的切分;
// 模型名自身含冒号的渠道, 把前缀设置成不含冒号即可避开歧义。
func directModelGroup(name string) (model.Group, bool) {
	for idx := 0; idx < len(name); idx++ {
		if name[idx] != ':' {
			continue
		}
		group, ok := directModelGroupByPrefix(name[:idx], name[idx+1:])
		if ok {
			return group, true
		}
	}
	return model.Group{}, false
}

// directModelGroupByPrefix 按生效前缀与模型名聚合各渠道的可用授权, 构造故障转移分组。
// 成员按渠道主键与凭据主键定序, 同一 ID 每次解析得到同一成员顺序, 故障转移次序稳定。
// 分组与成员都是读取时的临时构造, 不落库: 路由状态由直连 ID 哈希出的稳定主键保存,
// 冷却与亲和因此跨请求延续; 成员主键直接取授权主键, 凭据增删后顺序自然重排。
func directModelGroupByPrefix(prefix, modelName string) (model.Group, bool) {
	if prefix == "" || modelName == "" {
		return model.Group{}, false
	}

	type directCandidate struct {
		channel model.Channel
		grant   model.ChannelGrant
		keyName string // 缓存中的授权不带关联, 凭据名称在此随候选一并取出。
	}
	candidates := make([]directCandidate, 0, 2)
	for _, grant := range channelGrantCache.GetAll() {
		channelModel, modelOK := channelModelCache.Get(grant.ChannelModelID)
		if !modelOK || channelModel.Name != modelName {
			continue
		}
		channel, channelOK := channelCache.Get(channelModel.ChannelID)
		if !channelOK || !channel.Enabled || effectiveModelPrefix(channel) != prefix {
			continue
		}
		channelKey, keyOK := channelKeyCache.Get(grant.ChannelKeyID)
		if !keyOK || !channelKey.Enabled {
			continue
		}
		candidates = append(candidates, directCandidate{channel: channel, grant: grant, keyName: channelKey.Name})
	}
	if len(candidates) == 0 {
		return model.Group{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].channel.ID != candidates[j].channel.ID {
			return candidates[i].channel.ID < candidates[j].channel.ID
		}
		return candidates[i].grant.ChannelKeyID < candidates[j].grant.ChannelKeyID
	})

	id := prefix + directModelSeparator + modelName
	groupID := directGroupID(id)
	items := make([]model.GroupItem, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, model.GroupItem{
			ID:             candidate.grant.ID,
			GroupID:        groupID,
			ChannelGrantID: candidate.grant.ID,
			Priority:       len(items) + 1,
			ChannelID:      candidate.channel.ID,
			ChannelName:    candidate.channel.Name,
			ModelName:      modelName,
			KeyName:        candidate.keyName,
			Protocols:      candidate.grant.Protocols,
			Available:      true,
		})
	}
	return model.Group{
		ID:          groupID,
		Name:        id,
		Mode:        model.GroupModeFailover,
		RelayConfig: model.DefaultGroupRelayConfig(),
		Items:       items,
	}, true
}

// effectiveModelPrefix 返回渠道的直连前缀: 设置了模型前缀用前缀, 否则回退渠道名。
func effectiveModelPrefix(channel model.Channel) string {
	if channel.ModelPrefix != "" {
		return channel.ModelPrefix
	}
	return channel.Name
}

// directGroupID 生成直连分组的稳定主键: 基准值加直连 ID 的 FNV 哈希。
// 哈希空间远大于模型总数, 与显式分组主键冲突的概率可忽略。
func directGroupID(id string) int {
	hash := fnv.New32a()
	hash.Write([]byte(id))
	return directGroupIDBase + int(hash.Sum32())
}

// DirectModelNames 返回全部启用渠道当前提供的直连模型 ID, 按名称定序并去重。
// ID 由生效前缀与渠道模型名拼出: 同前缀渠道的同名模型共用一个 ID; 不检查凭据与授权,
// 与显式分组按名称列出的口径一致, 凭据停用只影响能否转发而不影响模型是否可见。
func DirectModelNames() []string {
	modelsByChannel := make(map[int]map[string]bool)
	for _, channelModel := range channelModelCache.GetAll() {
		names := modelsByChannel[channelModel.ChannelID]
		if names == nil {
			names = make(map[string]bool)
			modelsByChannel[channelModel.ChannelID] = names
		}
		names[channelModel.Name] = true
	}
	names := make([]string, 0, 8)
	seen := make(map[string]bool)
	for _, channel := range channelCache.GetAll() {
		if !channel.Enabled {
			continue
		}
		prefix := effectiveModelPrefix(channel)
		for modelName := range modelsByChannel[channel.ID] {
			id := prefix + directModelSeparator + modelName
			if seen[id] {
				continue
			}
			seen[id] = true
			names = append(names, id)
		}
	}
	sort.Strings(names)
	return names
}

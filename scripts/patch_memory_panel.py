import pathlib

p = pathlib.Path('frontend/src/components/features/MemoryPanel.vue')
t = p.read_text(encoding='utf-8')

head_old = """// 记忆面板：四条记忆流的清单 + 按类别筛选 + 单条删除。
// 记忆库是明文的，用户必须能看见里面到底存了什么、并能删掉不该在的东西。"""

head_new = """// 记忆面板：四条记忆流的清单 + 按类别筛选 + 手动新增 + 单条删除。
// 记忆库是明文的，用户必须能看见里面到底存了什么、能补一条、也能删掉不该在的东西。
// 新增走后端同一套密钥扫描——界面写入不能成为绕过检查的后门。"""

assert head_old in t
t = t.replace(head_old, head_new)

state_old = """/** 正在删除的记忆 id（按行锁按钮） */
const deleting = ref('')"""

state_new = """/** 正在删除的记忆 id（按行锁按钮） */
const deleting = ref('')
/** 新增表单：类别跟当前筛选走，用户在哪一类下就在哪一类里加 */
const adding = ref(false)
const saving = ref(false)
const draftKind = ref('memory')
const draftText = ref('')
/** 新增时可选的四类（不受当前筛选限制） */
const ALL_KINDS = ['memory', 'user', 'project', 'failure'] as const"""

assert state_old in t
t = t.replace(state_old, state_new)

onmount_old = """onMounted(() => {"""
onmount_new = """function startAdding() {
  draftKind.value = kind.value || 'memory'
  draftText.value = ''
  adding.value = true
}

async function submitDraft() {
  const text = draftText.value.trim()
  if (!text || saving.value) return
  saving.value = true
  try {
    await api.memory.save(draftKind.value, text)
    toast.ok(t('featurePanel.memorySaved'))
    adding.value = false
    draftText.value = ''
    await reload()
  } catch (e) {
    toast.bad(errorText(e))
  } finally {
    saving.value = false
  }
}

onMounted(() => {"""

assert onmount_old in t
t = t.replace(onmount_old, onmount_new, 1)

empty_old = """    <EmptyState
      v-else-if="!totalCount"
      icon="brain"
      :title="t('featurePanel.memoryEmpty')"
      :desc="t('featurePanel.memoryEmptyHint')"
    />"""

empty_new = """    <template v-else-if="!totalCount">
      <div v-if="adding" class="create-box">
        <el-select v-model="draftKind" size="small" class="kind-sel">
          <el-option
            v-for="k in ALL_KINDS"
            :key="k"
            :value="k"
            :label="t(`featurePanel.memoryKind.${k}`)"
          />
        </el-select>
        <el-input
          v-model="draftText"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 8 }"
          :placeholder="t('featurePanel.memoryAddPlaceholder')"
        />
        <div class="create-acts">
          <el-button
            size="small"
            type="primary"
            :loading="saving"
            :disabled="!draftText.trim() || saving"
            @click="submitDraft"
          >
            {{ t('common.confirm') }}
          </el-button>
          <el-button size="small" :disabled="saving" @click="adding = false">
            {{ t('common.cancel') }}
          </el-button>
        </div>
      </div>
      <template v-else>
        <EmptyState
          icon="brain"
          :title="t('featurePanel.memoryEmpty')"
          :desc="t('featurePanel.memoryEmptyHint')"
        />
        <el-button size="small" class="mk-btn" @click="startAdding">
          <AppIcon name="plus" size="ic-2xs" />
          <span class="ml-1">{{ t('featurePanel.memoryAdd') }}</span>
        </el-button>
      </template>
    </template>"""

assert empty_old in t
t = t.replace(empty_old, empty_new)

# 在 filters 之后插入新增表单（有内容时也能加）
filters_end_old = """      <div v-if="!shown.length" class="wb-card no-match">{{ t('featurePanel.memoryNoMatch') }}</div>"""

filters_end_new = """      <div v-if="adding" class="create-box">
        <el-select v-model="draftKind" size="small" class="kind-sel">
          <el-option
            v-for="k in ALL_KINDS"
            :key="k"
            :value="k"
            :label="t(`featurePanel.memoryKind.${k}`)"
          />
        </el-select>
        <el-input
          v-model="draftText"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 8 }"
          :placeholder="t('featurePanel.memoryAddPlaceholder')"
        />
        <div class="create-acts">
          <el-button
            size="small"
            type="primary"
            :loading="saving"
            :disabled="!draftText.trim() || saving"
            @click="submitDraft"
          >
            {{ t('common.confirm') }}
          </el-button>
          <el-button size="small" :disabled="saving" @click="adding = false">
            {{ t('common.cancel') }}
          </el-button>
        </div>
      </div>
      <el-button v-else size="small" class="mk-btn" @click="startAdding">
        <AppIcon name="plus" size="ic-2xs" />
        <span class="ml-1">{{ t('featurePanel.memoryAdd') }}</span>
      </el-button>

      <div v-if="!shown.length" class="wb-card no-match">{{ t('featurePanel.memoryNoMatch') }}</div>"""

assert filters_end_old in t
t = t.replace(filters_end_old, filters_end_new)

style_old = """.mem-wrap {
  display: flex;
  flex-direction: column;"""

style_new = """.create-box {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.create-acts {
  display: flex;
  gap: var(--wb-sp-2);
}
.kind-sel {
  width: calc(160px * var(--wb-vw-scale));
}
.mk-btn {
  align-self: flex-start;
}
.mem-wrap {
  display: flex;
  flex-direction: column;"""

assert style_old in t
t = t.replace(style_old, style_new, 1)

p.write_text(t, encoding='utf-8')
print('MemoryPanel ok')

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useIdentity } from '@/stores/identity'
import { useToasts } from '@/stores/toasts'
import { api } from '@/lib/api'
import Card from '@/components/Card.vue'
import Button from '@/components/Button.vue'
import Input from '@/components/Input.vue'
import Field from '@/components/Field.vue'
import HashChip from '@/components/HashChip.vue'
import KV from '@/components/KV.vue'
import EmptyState from '@/components/EmptyState.vue'
import { KeyRound, RotateCw, Download, Upload, Trash2, ShieldCheck, Eye, EyeOff, Copy } from 'lucide-vue-next'
import { copyToClipboard } from '@/lib/format'

const identity = useIdentity()
const toasts = useToasts()

const hasIdentity = computed(() => !!identity.identity)

// -------- Create ----------
const newTenant = ref('default')
const newClient = ref('desktop-1')
const newKeyID = ref('ed25519-1')
const busyCreate = ref(false)
async function create() {
  if (!newTenant.value || !newClient.value || !newKeyID.value) {
    toasts.error('缺少字段', '租户、客户端、key_id 都必须填写')
    return
  }
  busyCreate.value = true
  try {
    await identity.generate(newTenant.value, newClient.value, newKeyID.value)
    toasts.success('身份已生成')
  } catch (e: any) {
    toasts.error('生成失败', String(e?.message ?? e))
  } finally {
    busyCreate.value = false
  }
}

// -------- Rotate ----------
const rotateKeyID = ref('')
const busyRotate = ref(false)
async function rotate() {
  if (!rotateKeyID.value) {
    toasts.error('填入新的 key_id', '新旧 key_id 必须不同，避免服务端混淆')
    return
  }
  if (identity.identity && rotateKeyID.value === identity.identity.key_id) {
    toasts.error('key_id 必须与当前不同')
    return
  }
  busyRotate.value = true
  try {
    await identity.rotate(rotateKeyID.value)
    toasts.success('密钥已轮换', '服务端下次提交后即可识别新的 key_id')
    rotateKeyID.value = ''
  } catch (e: any) {
    toasts.error('轮换失败', String(e?.message ?? e))
  } finally {
    busyRotate.value = false
  }
}

// -------- Import ----------
const impTenant = ref('default')
const impClient = ref('desktop-1')
const impKeyID = ref('ed25519-1')
const impPriv = ref('')
const impShow = ref(false)
const busyImport = ref(false)
async function importKey() {
  if (!impTenant.value || !impClient.value || !impKeyID.value || !impPriv.value) {
    toasts.error('缺少字段')
    return
  }
  busyImport.value = true
  try {
    await identity.importKey(impTenant.value, impClient.value, impKeyID.value, impPriv.value.trim())
    toasts.success('密钥已导入')
    impPriv.value = ''
  } catch (e: any) {
    toasts.error('导入失败', String(e?.message ?? e))
  } finally {
    busyImport.value = false
  }
}

// -------- Export / reveal ----------
const priv = ref('')
const showPriv = ref(false)
async function reveal() {
  if (!confirm('即将在本机显示当前私钥的 base64 值。请确认周围没有旁观者或录屏。确定继续？')) return
  try {
    priv.value = await api.exportPrivateKey()
    showPriv.value = true
  } catch (e: any) {
    toasts.error('读取私钥失败', String(e?.message ?? e))
  }
}
async function copyPriv() {
  if (!priv.value) return
  await copyToClipboard(priv.value)
  toasts.success('已复制')
}

// -------- Clear ----------
async function clear() {
  if (!confirm('清除身份将移除本地保存的私钥。未导出备份的话无法再用此身份签名新 claim。确定继续？')) return
  try {
    await identity.clear()
    priv.value = ''
    showPriv.value = false
    toasts.success('已清除本地身份')
  } catch (e: any) {
    toasts.error('清除失败', String(e?.message ?? e))
  }
}
</script>

<template>
  <div class="flex flex-col gap-5 max-w-[1100px] mx-auto">
    <!-- Current identity -->
    <Card>
      <template #title>
        <div class="flex items-center gap-2">
          <ShieldCheck :size="15" class="text-accent" />
          <h3 class="text-[14px] font-semibold tracking-[-0.01em] text-ink-800 dark:text-ink-100">当前身份</h3>
        </div>
      </template>
      <template v-if="hasIdentity" #actions>
        <Button size="sm" variant="subtle" @click="reveal">
          <Eye :size="13" /> 查看私钥
        </Button>
        <Button size="sm" variant="danger" @click="clear">
          <Trash2 :size="13" /> 清除
        </Button>
      </template>

      <template v-if="hasIdentity">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <KV label="tenant_id"><span class="font-mono text-[12px]">{{ identity.identity!.tenant_id }}</span></KV>
          <KV label="client_id"><span class="font-mono text-[12px]">{{ identity.identity!.client_id }}</span></KV>
          <KV label="key_id"><span class="font-mono text-[12px]">{{ identity.identity!.key_id }}</span></KV>
          <KV label="算法"><span class="text-[12px]">Ed25519</span></KV>
          <KV label="public_key (base64)" class="sm:col-span-2">
            <HashChip :value="identity.identity!.public_key_b64" :head="16" :tail="10" />
          </KV>
        </div>

        <div v-if="showPriv" class="mt-4 rounded-xl border-2 border-warn/30 bg-warn/5 p-3">
          <div class="flex items-center justify-between mb-2">
            <div class="flex items-center gap-1.5 text-[12px] text-warn">
              <Eye :size="13" /> 私钥（Ed25519 seed，base64）
            </div>
            <div class="flex items-center gap-1">
              <button class="text-ink-500 hover:text-ink-800 text-[11.5px] flex items-center gap-1" @click="copyPriv">
                <Copy :size="12" /> 复制
              </button>
              <button class="text-ink-500 hover:text-ink-800 text-[11.5px] flex items-center gap-1 ml-3" @click="showPriv = false">
                <EyeOff :size="12" /> 隐藏
              </button>
            </div>
          </div>
          <pre class="font-mono text-[11.5px] text-ink-800 dark:text-ink-100 break-all whitespace-pre-wrap">{{ priv }}</pre>
          <p class="mt-2 text-[11px] text-ink-500">
            记录这串 base64 到安全的密码管理器即可。下次通过「导入现有密钥」可还原身份。
          </p>
        </div>
      </template>

      <EmptyState
        v-else
        title="尚未配置身份"
        hint="TrustDB 用 Ed25519 密钥对来证明是你签署的 claim。下方可新建或导入一个现有密钥。"
        :icon="KeyRound"
      />
    </Card>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
      <!-- Create -->
      <Card :title="hasIdentity ? '新建（将覆盖当前身份）' : '新建身份'" subtitle="随机生成 Ed25519 密钥对">
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <Field label="tenant_id"><Input v-model="newTenant" /></Field>
          <Field label="client_id"><Input v-model="newClient" /></Field>
          <Field label="key_id"><Input v-model="newKeyID" /></Field>
        </div>
        <div class="mt-4 flex justify-end">
          <Button :loading="busyCreate" @click="create">
            <KeyRound :size="13" /> 生成身份
          </Button>
        </div>
      </Card>

      <!-- Import -->
      <Card title="导入现有密钥" subtitle="粘贴 Ed25519 seed (base64)，32 字节">
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <Field label="tenant_id"><Input v-model="impTenant" /></Field>
          <Field label="client_id"><Input v-model="impClient" /></Field>
          <Field label="key_id"><Input v-model="impKeyID" /></Field>
        </div>
        <Field label="私钥 seed (base64)" hint="留意本地安全，导入后会覆盖当前身份" class="mt-3">
          <div class="relative">
            <Input
              v-model="impPriv"
              :type="impShow ? 'text' : 'password'"
              :mono="true"
              placeholder="e.g. p5F2…"
            />
            <button
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 text-ink-400 hover:text-ink-700"
              @click="impShow = !impShow"
            >
              <component :is="impShow ? EyeOff : Eye" :size="13" />
            </button>
          </div>
        </Field>
        <div class="mt-4 flex justify-end">
          <Button variant="subtle" :loading="busyImport" @click="importKey">
            <Upload :size="13" /> 导入
          </Button>
        </div>
      </Card>
    </div>

    <!-- Rotate -->
    <Card v-if="hasIdentity" title="轮换密钥" subtitle="保留 tenant/client，生成新的 key_id 和新的密钥对">
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <Field label="新 key_id" hint="服务端见到新 key 后会更新验签公钥（需要服务器允许）">
          <Input v-model="rotateKeyID" placeholder="ed25519-2" />
        </Field>
      </div>
      <div class="mt-4 flex justify-end gap-2">
        <Button variant="subtle" :loading="busyRotate" @click="rotate">
          <RotateCw :size="13" /> 轮换
        </Button>
      </div>
    </Card>
  </div>
</template>

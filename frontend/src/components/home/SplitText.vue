<template>
  <component :is="tag" class="split-text" :style="{ '--n': total }">
    <span class="sr-only">{{ plain }}</span>
    <span class="split-inner" aria-hidden="true">
      <span v-for="(line, li) in lines" :key="li" class="split-line">
        <template v-for="(token, ti) in line" :key="ti">
          <span v-if="token.space" class="split-space">{{ ' ' }}</span>
          <span v-else class="split-ch" :class="{ 'is-hl': token.highlight }" :style="{ '--i': token.index }">{{ token.text }}</span>
        </template>
      </span>
    </span>
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { splitTokens } from './splitTokens'

const props = withDefaults(defineProps<{ text: string; tag?: string }>(), { tag: 'span' })

const plain = computed(() => props.text.replace(/\[\[|\]\]/g, ''))
const lines = computed(() => {
  let offset = 0
  return props.text.split('\n').map(line => {
    const tokens = splitTokens(line).map(token => ({ ...token, index: token.index < 0 ? -1 : token.index + offset }))
    offset += tokens.filter(token => !token.space).length
    return tokens
  })
})
const total = computed(() => lines.value.reduce((sum, line) => sum + line.filter(token => !token.space).length, 0))
</script>

<template>
  <AppLayout>
    <div class="space-y-6">
      <SmartOpsNav />
      <HarvestGatewayBorrowPanel start-expanded />
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
import AppLayout from '@/components/layout/AppLayout.vue'
import SmartOpsNav from '@/components/admin/operations/SmartOpsNav.vue'
import HarvestGatewayBorrowPanel from '@/components/admin/HarvestGatewayBorrowPanel.vue'
</script>

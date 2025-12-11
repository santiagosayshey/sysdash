<script>
  import { onMount, onDestroy } from 'svelte';

  let stats = {
    hostname: '...',
    uptime: 0,
    cpu: { model: '', percent: [] },
    memory: { used: 0, total: 0, usedPercent: 0 },
    disk: { used: 0, total: 0, usedPercent: 0, path: '' },
    network: [],
    gpu: null
  };
  let error = null;
  let ws;

  function connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/ws`;

    ws = new WebSocket(wsUrl);
    ws.onopen = () => { error = null; };
    ws.onmessage = (event) => { stats = JSON.parse(event.data); };
    ws.onerror = () => { error = 'Connection error'; };
    ws.onclose = () => { setTimeout(connect, 2000); };
  }

  function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  }

  function formatUptime(seconds) {
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    if (days > 0) return `${days}d ${hours}h`;
    const mins = Math.floor((seconds % 3600) / 60);
    if (hours > 0) return `${hours}h ${mins}m`;
    return `${mins}m`;
  }

  function avgCpu(percent) {
    if (!percent || percent.length === 0) return 0;
    return Math.round(percent.reduce((a, b) => a + b, 0) / percent.length);
  }

  onMount(() => connect());
  onDestroy(() => { if (ws) ws.close(); });
</script>

<div class="min-h-screen bg-neutral-900 text-neutral-100 p-6" style="font-family: '0xProto', monospace;">
  {#if error}
    <div class="text-red-400">{error}</div>
  {:else}
    <!-- Header -->
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-xl font-semibold">{stats.hostname}</h1>
      <span class="text-sm text-neutral-500">up {formatUptime(stats.uptime)}</span>
    </div>

    <!-- Cards Grid -->
    <div class="grid grid-cols-2 gap-4">
      <!-- CPU -->
      <div class="bg-neutral-800 rounded-lg p-4">
        <div class="flex items-center gap-2 mb-2">
          <span class="text-lg text-neutral-400">CPU</span>
          {#if stats.cpu.model}
            <span class="text-xs text-neutral-500">{stats.cpu.model}</span>
          {/if}
        </div>
        <div class="flex items-baseline gap-2 mb-2">
          <span class="text-3xl font-bold">{avgCpu(stats.cpu.percent)}%</span>
          <span class="text-sm text-neutral-500">{stats.cpu.cores} cores / {stats.cpu.threads} threads</span>
        </div>
        <div class="h-2 bg-neutral-700 rounded-full overflow-hidden">
          <div class="h-full bg-blue-500 transition-all duration-300" style="width: {avgCpu(stats.cpu.percent)}%"></div>
        </div>
      </div>

      <!-- Memory -->
      <div class="bg-neutral-800 rounded-lg p-4">
        <div class="flex items-center gap-2 mb-2">
          <span class="text-lg text-neutral-400">Memory</span>
          <span class="text-xs text-neutral-500">{formatBytes(stats.memory.used)} / {formatBytes(stats.memory.total)}</span>
        </div>
        <div class="text-3xl font-bold mb-2">{Math.round(stats.memory.usedPercent)}%</div>
        <div class="h-2 bg-neutral-700 rounded-full overflow-hidden">
          <div class="h-full bg-emerald-500 transition-all duration-300" style="width: {stats.memory.usedPercent}%"></div>
        </div>
      </div>

      <!-- Disk -->
      <div class="bg-neutral-800 rounded-lg p-4">
        <div class="flex items-center gap-2 mb-2">
          <span class="text-lg text-neutral-400">Disk</span>
          <span class="text-xs text-neutral-500">{formatBytes(stats.disk.used)} / {formatBytes(stats.disk.total)}{#if stats.disk.path && stats.disk.path !== '/'} · {stats.disk.path}{/if}</span>
        </div>
        <div class="text-3xl font-bold mb-2">{Math.round(stats.disk.usedPercent)}%</div>
        <div class="h-2 bg-neutral-700 rounded-full overflow-hidden">
          <div class="h-full bg-amber-500 transition-all duration-300" style="width: {stats.disk.usedPercent}%"></div>
        </div>
      </div>

      <!-- Network -->
      {#if stats.network && stats.network.length > 0}
        <div class="bg-neutral-800 rounded-lg p-4">
          <div class="flex items-center gap-2 mb-2">
            <span class="text-lg text-neutral-400">Network</span>
            <span class="text-xs text-neutral-500">{stats.network[0].name}</span>
          </div>
          <div class="text-xl mb-1">
            <span class="text-emerald-400">↑</span> {formatBytes(stats.network[0].bytesSent)}
          </div>
          <div class="text-xl">
            <span class="text-blue-400">↓</span> {formatBytes(stats.network[0].bytesRecv)}
          </div>
        </div>
      {/if}

      <!-- GPU -->
      {#if stats.gpu}
        <div class="bg-neutral-800 rounded-lg p-4 col-span-2">
          <div class="flex items-center gap-2 mb-2">
            <span class="text-lg text-neutral-400">GPU</span>
            {#if stats.gpu.name}
              <span class="text-xs text-neutral-500">{stats.gpu.name}</span>
            {/if}
          </div>
          {#if stats.gpu.memoryTotal > 4294967296}
            <div class="text-3xl font-bold mb-2">{Math.round(stats.gpu.usedPercent)}%</div>
            <div class="h-2 bg-neutral-700 rounded-full overflow-hidden mb-2">
              <div class="h-full bg-purple-500 transition-all duration-300" style="width: {stats.gpu.usedPercent}%"></div>
            </div>
            <div class="text-xs text-neutral-500">{stats.gpu.temperature}°C · {formatBytes(stats.gpu.memoryUsed)} / {formatBytes(stats.gpu.memoryTotal)}</div>
          {/if}
        </div>
      {/if}
    </div>
  {/if}
</div>

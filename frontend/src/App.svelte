<script>
  import { onMount, onDestroy } from 'svelte';

  let stats = null;
  let error = null;
  let connected = false;
  let ws;

  function connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/ws`;

    ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      connected = true;
      error = null;
    };

    ws.onmessage = (event) => {
      stats = JSON.parse(event.data);
    };

    ws.onerror = () => {
      error = 'WebSocket error';
    };

    ws.onclose = () => {
      connected = false;
      // Reconnect after 2 seconds
      setTimeout(connect, 2000);
    };
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
    const mins = Math.floor((seconds % 3600) / 60);
    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${mins}m`;
    return `${mins}m`;
  }

  onMount(() => {
    connect();
  });

  onDestroy(() => {
    if (ws) ws.close();
  });
</script>

<main class="min-h-screen bg-slate-900 text-white p-4">
  {#if error}
    <div class="text-red-400 text-center p-4">
      Error: {error}
    </div>
  {:else if !stats}
    <div class="text-slate-400 text-center p-4">
      Loading...
    </div>
  {:else}
    <div class="max-w-4xl mx-auto space-y-4">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <h1 class="text-xl font-semibold">{stats.hostname}</h1>
        <span class="text-sm text-slate-400">
          {stats.os}/{stats.arch} &middot; up {formatUptime(stats.uptime)}
        </span>
      </div>

      <!-- Stats Grid -->
      <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
        <!-- CPU -->
        <div class="bg-slate-800 rounded-lg p-4">
          <div class="text-sm text-slate-400 mb-1">CPU</div>
          <div class="text-2xl font-bold">
            {Math.round(stats.cpu.percent.reduce((a, b) => a + b, 0) / stats.cpu.percent.length)}%
          </div>
          <div class="text-xs text-slate-500">{stats.cpu.cores} cores</div>
          <div class="mt-2 h-2 bg-slate-700 rounded-full overflow-hidden">
            <div
              class="h-full bg-blue-500 transition-all duration-300"
              style="width: {stats.cpu.percent.reduce((a, b) => a + b, 0) / stats.cpu.percent.length}%"
            ></div>
          </div>
        </div>

        <!-- Memory -->
        <div class="bg-slate-800 rounded-lg p-4">
          <div class="text-sm text-slate-400 mb-1">Memory</div>
          <div class="text-2xl font-bold">{Math.round(stats.memory.usedPercent)}%</div>
          <div class="text-xs text-slate-500">
            {formatBytes(stats.memory.used)} / {formatBytes(stats.memory.total)}
          </div>
          <div class="mt-2 h-2 bg-slate-700 rounded-full overflow-hidden">
            <div
              class="h-full bg-green-500 transition-all duration-300"
              style="width: {stats.memory.usedPercent}%"
            ></div>
          </div>
        </div>

        <!-- Disk -->
        <div class="bg-slate-800 rounded-lg p-4">
          <div class="text-sm text-slate-400 mb-1">Disk</div>
          <div class="text-2xl font-bold">{Math.round(stats.disk.usedPercent)}%</div>
          <div class="text-xs text-slate-500">
            {formatBytes(stats.disk.used)} / {formatBytes(stats.disk.total)}
          </div>
          <div class="mt-2 h-2 bg-slate-700 rounded-full overflow-hidden">
            <div
              class="h-full bg-amber-500 transition-all duration-300"
              style="width: {stats.disk.usedPercent}%"
            ></div>
          </div>
        </div>

        <!-- Network -->
        <div class="bg-slate-800 rounded-lg p-4">
          <div class="text-sm text-slate-400 mb-1">Network</div>
          {#if stats.network && stats.network.length > 0}
            {@const primary = stats.network[0]}
            <div class="text-sm">
              <span class="text-green-400">&uarr;</span> {formatBytes(primary.bytesSent)}
            </div>
            <div class="text-sm">
              <span class="text-blue-400">&darr;</span> {formatBytes(primary.bytesRecv)}
            </div>
            <div class="text-xs text-slate-500 mt-1">{primary.name}</div>
          {:else}
            <div class="text-slate-500 text-sm">No data</div>
          {/if}
        </div>
      </div>

      <!-- Per-core CPU (collapsible or always visible for small core counts) -->
      {#if stats.cpu.cores <= 8}
        <div class="bg-slate-800 rounded-lg p-4">
          <div class="text-sm text-slate-400 mb-2">CPU Cores</div>
          <div class="grid grid-cols-4 md:grid-cols-8 gap-2">
            {#each stats.cpu.percent as pct, i}
              <div class="text-center">
                <div class="text-xs text-slate-500 mb-1">{i}</div>
                <div class="h-16 bg-slate-700 rounded relative overflow-hidden">
                  <div
                    class="absolute bottom-0 w-full bg-blue-500 transition-all duration-300"
                    style="height: {pct}%"
                  ></div>
                </div>
                <div class="text-xs mt-1">{Math.round(pct)}%</div>
              </div>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  {/if}
</main>

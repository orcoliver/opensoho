<script>
    import { pageTitle } from "@/stores/app";
    import { addSuccessToast, addErrorToast } from "@/stores/toasts";
    import ApiClient from "@/utils/ApiClient";
    import PageWrapper from "@/components/base/PageWrapper.svelte";
    import { link } from "svelte-spa-router";

    export let params = {};

    let device = null;
    let deviceStatus = null;
    let wifis = [];
    let radios = [];
    let leds = [];
    let dawnConfigured = false;
    let isLoading = true;
    let isPushing = false;
    let isRebooting = false;
    let profiles = [];
    let selectedProfileId = "";
    let isApplyingProfile = false;

    $: if (params.deviceId) {
        loadDevice(params.deviceId);
    }

    async function loadDevice(id) {
        isLoading = true;
        try {
            device = await ApiClient.collection("devices").getOne(id);
            $pageTitle = device.name || "Device Details";

            try {
                deviceStatus = await ApiClient.send(`/api/ssh/device/${id}/status`, { method: "GET" });
            } catch {
                deviceStatus = { connected: false, status: "offline" };
            }

            // Load related data in parallel
            const [wifiList, radioList, ledList, dawnList] = await Promise.allSettled([
                device.wifis?.length
                    ? ApiClient.collection("wifi").getList(1, 50, { filter: device.wifis.map(id => `id='${id}'`).join("||")})
                    : Promise.resolve({ items: [] }),
                ApiClient.collection("radios").getList(1, 10, { filter: `device='${id}'`, sort: "radio" }),
                device.leds?.length
                    ? ApiClient.collection("leds").getList(1, 50, { filter: device.leds.map(id => `id='${id}'`).join("||")})
                    : Promise.resolve({ items: [] }),
                ApiClient.collection("dawn").getList(1, 1),
            ]);

            wifis = wifiList.status === "fulfilled" ? wifiList.value.items : [];
            radios = radioList.status === "fulfilled" ? radioList.value.items : [];
            leds = ledList.status === "fulfilled" ? ledList.value.items : [];
            dawnConfigured = dawnList.status === "fulfilled" && dawnList.value.totalItems > 0;

            profiles = await ApiClient.collection("device_profile").getFullList({ sort: "name" });
            selectedProfileId = device.profile || "";
        } catch (err) {
            ApiClient.error(err);
        }
        isLoading = false;
    }

    async function pushConfig() {
        if (!device) return;
        isPushing = true;
        try {
            const resp = await ApiClient.send(`/api/ssh/push-config/${device.id}`, { method: "POST" });
            addSuccessToast(resp.message || "Config pushed!");
            await loadDevice(device.id);
        } catch (err) {
            addErrorToast(err?.data?.error || err?.message || "Config push failed");
        }
        isPushing = false;
    }

    async function rebootDevice() {
        if (!device || !confirm(`Reboot ${device.name}?`)) return;
        isRebooting = true;
        try {
            await ApiClient.send(`/api/ssh/device/${device.id}/reboot`, { method: "POST" });
            addSuccessToast(`${device.name} is rebooting...`);
        } catch (err) {
            addErrorToast(err?.data?.error || "Reboot failed");
        }
        isRebooting = false;
    }

    async function applyProfileAndPush() {
        if (!device) return;
        isApplyingProfile = true;
        try {
            await ApiClient.collection("devices").update(device.id, { profile: selectedProfileId || null });
            const resp = await ApiClient.send(`/api/ssh/push-config/${device.id}`, { method: "POST" });
            addSuccessToast(resp.message || "Profile applied and config pushed!");
            await loadDevice(device.id);
        } catch (err) {
            addErrorToast(err?.data?.error || err?.message || "Apply profile failed");
        }
        isApplyingProfile = false;
    }

    function profileSummary(id) {
        const p = profiles.find(x => x.id === id);
        if (!p) return "";
        const parts = [];
        if (p.disable_firewall) parts.push("firewall disabled");
        if (p.disable_dnsmasq) parts.push("dnsmasq disabled");
        if (p.igmp_snooping) parts.push("IGMP snooping");
        parts.push(`LAN: ${p.lan_proto}`);
        return parts.join(" · ");
    }

    function formatDate(dateStr) {
        if (!dateStr) return '—';
        try {
            return new Date(dateStr).toLocaleString();
        } catch {
            return dateStr;
        }
    }
</script>

<PageWrapper>
    <header class="page-header">
        <nav class="breadcrumbs">
            <a href="/ssh/devices" class="breadcrumb-item" use:link>SSH Management</a>
            <div class="breadcrumb-item">{device?.name || 'Device'}</div>
        </nav>
        {#if device}
            <div class="btns-group">
                <button
                    type="button"
                    class="btn btn-outline"
                    class:btn-loading={isPushing}
                    disabled={isPushing}
                    on:click={pushConfig}
                >
                    <i class="ri-upload-cloud-line" />
                    <span class="txt">Push Config</span>
                </button>
                <button
                    type="button"
                    class="btn btn-danger btn-outline"
                    class:btn-loading={isRebooting}
                    disabled={isRebooting}
                    on:click={rebootDevice}
                >
                    <i class="ri-restart-line" />
                    <span class="txt">Reboot</span>
                </button>
            </div>
        {/if}
    </header>

    <div class="wrapper">
        {#if isLoading}
            <div class="loader" />
        {:else if device}
            <div class="grid">
                <!-- Device Info Card -->
                <div class="col-lg-6">
                    <div class="panel detail-panel">
                        <div class="panel-header">
                            <h6><i class="ri-router-line" /> Device Info</h6>
                        </div>
                        <div class="panel-content">
                            <dl class="detail-list">
                                <div class="detail-row">
                                    <dt>Name</dt>
                                    <dd class="txt-bold">{device.name || '—'}</dd>
                                </div>
                                <div class="detail-row">
                                    <dt>Model</dt>
                                    <dd>{device.model || '—'}</dd>
                                </div>
                                <div class="detail-row">
                                    <dt>MAC Address</dt>
                                    <dd><code>{device.mac_address || '—'}</code></dd>
                                </div>
                                <div class="detail-row">
                                    <dt>IP Address</dt>
                                    <dd><code>{device.ip_address || '—'}</code></dd>
                                </div>
                                <div class="detail-row">
                                    <dt>Radios</dt>
                                    <dd>{device.numradios || 0}</dd>
                                </div>
                                <div class="detail-row">
                                    <dt>Created</dt>
                                    <dd>{formatDate(device.created)}</dd>
                                </div>
                                <div class="detail-row">
                                    <dt>Updated</dt>
                                    <dd>{formatDate(device.updated)}</dd>
                                </div>
                            </dl>
                            <div class="profile-selector m-t-sm">
                                <label class="txt-sm txt-hint">Device Profile</label>
                                <div class="inline-flex flex-gap-5 m-t-xs">
                                    <select bind:value={selectedProfileId} class="select-sm">
                                        <option value="">— No profile —</option>
                                        {#each profiles as p}
                                            <option value={p.id}>{p.name}</option>
                                        {/each}
                                    </select>
                                    <button
                                        type="button"
                                        class="btn btn-sm btn-info"
                                        class:btn-loading={isApplyingProfile}
                                        disabled={isApplyingProfile}
                                        on:click={applyProfileAndPush}
                                    >
                                        <i class="ri-upload-cloud-line" />
                                        <span class="txt">Apply &amp; Push</span>
                                    </button>
                                </div>
                                {#if selectedProfileId}
                                    <p class="txt-hint txt-xs m-t-xs">{profileSummary(selectedProfileId)}</p>
                                {/if}
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Health Card -->
                <div class="col-lg-6">
                    <div class="panel detail-panel">
                        <div class="panel-header">
                            <h6><i class="ri-heart-pulse-line" /> Status</h6>
                        </div>
                        <div class="panel-content">
                            <dl class="detail-list">
                                <div class="detail-row">
                                    <dt>Health</dt>
                                    <dd>
                                        <span class="status-badge"
                                            class:status-healthy={device.health_status === 'healthy'}
                                            class:status-unhealthy={device.health_status === 'unhealthy'}
                                        >
                                            <span class="status-dot" />
                                            {device.health_status || 'unknown'}
                                        </span>
                                    </dd>
                                </div>
                                <div class="detail-row">
                                    <dt>Config Status</dt>
                                    <dd>
                                        <span class="config-badge"
                                            class:config-applied={device.config_status === 'applied'}
                                            class:config-error={device.config_status === 'error'}
                                            class:config-modified={device.config_status === 'modified'}
                                        >
                                            {device.config_status || 'pending'}
                                        </span>
                                    </dd>
                                </div>
                                <div class="detail-row">
                                    <dt>SSH Connected</dt>
                                    <dd>
                                        {#if deviceStatus?.connected}
                                            <span class="txt-success"><i class="ri-check-line" /> Yes</span>
                                        {:else}
                                            <span class="txt-danger"><i class="ri-close-line" /> No</span>
                                        {/if}
                                    </dd>
                                </div>
                                <div class="detail-row">
                                    <dt>SSH Status</dt>
                                    <dd class="txt-hint">{deviceStatus?.status || '—'}</dd>
                                </div>
                                {#if device.load_avg}
                                    <div class="detail-row">
                                        <dt>Load Average</dt>
                                        <dd>{device.load_avg}</dd>
                                    </div>
                                {/if}
                                {#if device.num_clients != null}
                                    <div class="detail-row">
                                        <dt>Connected Clients</dt>
                                        <dd class="txt-bold">{device.num_clients}</dd>
                                    </div>
                                {/if}
                            </dl>
                        </div>
                    </div>
                </div>

                <!-- Quick Actions -->
                <div class="col-lg-12">
                    <div class="panel detail-panel">
                        <div class="panel-header">
                            <h6><i class="ri-flashlight-line" /> Quick Actions</h6>
                        </div>
                        <div class="panel-content">
                            <div class="actions-grid">
                                <button type="button" class="action-btn" class:btn-loading={isPushing} on:click={pushConfig}>
                                    <i class="ri-upload-cloud-line" />
                                    <span>Push Config</span>
                                    <small>Apply pending changes via SSH</small>
                                </button>
                                <a href="/collections?collection=devices&recordId={device.id}" class="action-btn" use:link>
                                    <i class="ri-edit-line" />
                                    <span>Edit Record</span>
                                    <small>Modify device fields in PocketBase</small>
                                </a>
                                <a href="/collections?collection=wifi" class="action-btn" use:link>
                                    <i class="ri-wifi-line" />
                                    <span>WiFi Settings</span>
                                    <small>Manage SSIDs and passwords</small>
                                </a>
                                <a href="/ssh/leds" class="action-btn" use:link>
                                    <i class="ri-lightbulb-line" />
                                    <span>LED Config</span>
                                    <small>Manage LED triggers</small>
                                </a>
                                <a href="/ssh/dawn" class="action-btn" use:link>
                                    <i class="ri-router-line" />
                                    <span>DAWN Config</span>
                                    <small>{dawnConfigured ? 'Configured ✓' : 'Not configured'}</small>
                                </a>
                                <button type="button" class="action-btn action-danger" on:click={rebootDevice}>
                                    <i class="ri-restart-line" />
                                    <span>Reboot</span>
                                    <small>Restart this access point</small>
                                </button>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- WiFi Networks -->
                {#if wifis.length > 0}
                <div class="col-lg-6">
                    <div class="panel detail-panel">
                        <div class="panel-header">
                            <h6><i class="ri-wifi-line" /> WiFi Networks ({wifis.length})</h6>
                        </div>
                        <div class="panel-content">
                            {#each wifis as wifi}
                                <div class="wifi-row">
                                    <div class="wifi-info">
                                        <span class="txt-bold">{wifi.ssid}</span>
                                        <span class="label label-hint txt-sm">{wifi.encryption || '—'}</span>
                                    </div>
                                    <span class="label {wifi.enabled ? 'label-success' : 'label-hint'}">
                                        {wifi.enabled ? 'enabled' : 'disabled'}
                                    </span>
                                </div>
                            {/each}
                        </div>
                    </div>
                </div>
                {/if}

                <!-- Radios -->
                {#if radios.length > 0}
                <div class="col-lg-6">
                    <div class="panel detail-panel">
                        <div class="panel-header">
                            <h6><i class="ri-broadcast-line" /> Radios ({radios.length})</h6>
                        </div>
                        <div class="panel-content">
                            {#each radios as radio}
                                <div class="radio-row">
                                    <span class="txt-bold">radio{radio.radio}</span>
                                    <span class="label label-info">{radio.band} GHz</span>
                                    <span class="txt-hint txt-sm">
                                        {radio.auto_frequency ? 'auto' : `ch ${radio.channel}`}
                                        {radio.htmode ? `· ${radio.htmode}` : ''}
                                    </span>
                                </div>
                            {/each}
                        </div>
                    </div>
                </div>
                {/if}

                <!-- LEDs -->
                {#if leds.length > 0}
                <div class="col-lg-12">
                    <div class="panel detail-panel">
                        <div class="panel-header">
                            <h6><i class="ri-lightbulb-line" /> LEDs ({leds.length})</h6>
                        </div>
                        <div class="panel-content">
                            <div class="leds-grid">
                                {#each leds as led}
                                    <div class="led-item">
                                        <span class="txt-bold">{led.name}</span>
                                        <code class="txt-sm txt-hint">{led.led_name}</code>
                                        <span class="label label-hint">{led.trigger || 'none'}</span>
                                    </div>
                                {/each}
                            </div>
                        </div>
                    </div>
                </div>
                {/if}
            </div>
        {/if}
    </div>
</PageWrapper>

<style>
    .detail-panel .panel-header {
        padding: 12px 20px;
        border-bottom: 1px solid var(--baseAlt1Color);
    }
    .detail-panel .panel-header h6 {
        margin: 0;
        display: flex;
        align-items: center;
        gap: 8px;
    }
    .detail-panel .panel-content {
        padding: 16px 20px;
    }

    .detail-list {
        margin: 0;
        display: flex;
        flex-direction: column;
        gap: 0;
    }
    .detail-row {
        display: flex;
        justify-content: space-between;
        align-items: center;
        padding: 10px 0;
        border-bottom: 1px solid var(--baseAlt1Color);
    }
    .detail-row:last-child { border-bottom: none; }
    .detail-row dt {
        color: var(--txtHintColor);
        font-size: var(--smFontSize);
    }
    .detail-row dd {
        margin: 0;
        text-align: right;
    }

    /* Status Badges */
    .status-badge {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 3px 10px;
        border-radius: 30px;
        font-size: var(--xsFontSize);
        font-weight: 600;
        text-transform: capitalize;
        background: var(--baseAlt1Color);
        color: var(--txtHintColor);
    }
    .status-healthy { background: var(--successAltColor); color: #1a6b4a; }
    .status-unhealthy { background: var(--dangerAltColor); color: #a82a42; }

    .status-dot {
        width: 8px;
        height: 8px;
        border-radius: 50%;
        background: currentColor;
    }

    .config-badge {
        display: inline-block;
        padding: 3px 10px;
        border-radius: 30px;
        font-size: var(--xsFontSize);
        font-weight: 600;
        text-transform: capitalize;
        background: var(--baseAlt1Color);
        color: var(--txtHintColor);
    }
    .config-applied  { background: var(--successAltColor); color: #1a6b4a; }
    .config-error    { background: var(--dangerAltColor);  color: #a82a42; }
    .config-modified { background: var(--warningAltColor); color: #8a5a2a; }

    /* Actions Grid */
    .actions-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
        gap: 12px;
    }
    .action-btn {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 4px;
        padding: 20px 16px;
        border: 1px solid var(--baseAlt1Color);
        border-radius: var(--lgRadius);
        background: var(--baseColor);
        cursor: pointer;
        text-align: center;
        text-decoration: none;
        color: var(--txtPrimaryColor);
        transition: all var(--baseAnimationSpeed);
    }
    .action-btn:hover {
        border-color: var(--primaryColor);
        box-shadow: 0 2px 12px var(--shadowColor);
        transform: translateY(-1px);
    }
    .action-btn i {
        font-size: 24px;
        color: var(--primaryColor);
    }
    .action-btn span {
        font-weight: 600;
        font-size: var(--smFontSize);
    }
    .action-btn small {
        font-size: var(--xsFontSize);
        color: var(--txtHintColor);
        line-height: 1.3;
    }
    .action-danger:hover {
        border-color: var(--dangerColor);
    }
    .action-danger i {
        color: var(--dangerColor);
    }

    .txt-success { color: var(--successColor); }
    .txt-danger  { color: var(--dangerColor); }

    .wifi-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding: 8px 0;
        border-bottom: 1px solid var(--baseAlt1Color);
    }
    .wifi-row:last-child { border-bottom: none; }
    .wifi-info { display: flex; align-items: center; gap: 8px; }

    .radio-row {
        display: flex;
        align-items: center;
        gap: 10px;
        padding: 8px 0;
        border-bottom: 1px solid var(--baseAlt1Color);
    }
    .radio-row:last-child { border-bottom: none; }

    .leds-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
        gap: 10px;
    }
    .led-item {
        display: flex;
        flex-direction: column;
        gap: 4px;
        padding: 10px;
        border: 1px solid var(--baseAlt1Color);
        border-radius: var(--baseRadius);
    }

    .label {
        display: inline-block; padding: 2px 8px; border-radius: 30px;
        font-size: var(--xsFontSize); font-weight: 600; text-transform: capitalize;
    }
    .label-success { background: var(--successAltColor); color: #1a6b4a; }
    .label-hint    { background: var(--baseAlt1Color);   color: var(--txtHintColor); }
    .label-info    { background: var(--infoAltColor);    color: #2d6bb0; }

    .profile-selector { border-top: 1px solid var(--baseAlt1Color); padding-top: 12px; }
    .inline-flex { display: inline-flex; align-items: center; }
    .flex-gap-5 { gap: 5px; }
    .select-sm { height: 32px; font-size: var(--smFontSize); }
    .m-t-xs { margin-top: 4px; }
    .txt-xs { font-size: var(--xsFontSize); }
</style>

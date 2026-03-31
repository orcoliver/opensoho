<script>
    import { pageTitle } from "@/stores/app";
    import { addSuccessToast, addErrorToast } from "@/stores/toasts";
    import ApiClient from "@/utils/ApiClient";
    import PageWrapper from "@/components/base/PageWrapper.svelte";
    import { link } from "svelte-spa-router";

    $pageTitle = "Devices";

    let devices = [];
    let isLoading = true;

    // Discovery
    let isDiscovering = false;
    let discoveredDevices = [];
    let discoveryCidr = "192.168.1.0/24";
    let showDiscovery = false;

    // Adoption
    let adoptingDevice = null;
    let adoptPassword = "";
    let adoptImportConfig = false;
    let isAdopting = false;

    // Public key
    let publicKey = "";

    loadDevices();
    loadPublicKey();

    async function loadDevices() {
        isLoading = true;
        try {
            const records = await ApiClient.collection("devices").getFullList({
                sort: "name",
            });
            devices = records;
        } catch (err) {
            ApiClient.error(err);
        }
        isLoading = false;
    }

    async function loadPublicKey() {
        try {
            const resp = await ApiClient.send("/api/ssh/public-key", { method: "GET" });
            publicKey = resp.public_key || "";
        } catch (err) {
            console.warn("Could not load SSH public key:", err);
        }
    }

    async function discover() {
        isDiscovering = true;
        discoveredDevices = [];
        try {
            const resp = await ApiClient.send(`/api/ssh/discover?cidr=${encodeURIComponent(discoveryCidr)}`, {
                method: "GET",
            });
            discoveredDevices = resp.devices || [];
            if (discoveredDevices.length === 0) {
                addErrorToast("No devices found in " + discoveryCidr);
            } else {
                addSuccessToast(`Found ${discoveredDevices.length} device(s)`);
            }
        } catch (err) {
            ApiClient.error(err);
        }
        isDiscovering = false;
    }

    async function adoptDevice(device) {
        adoptingDevice = device;
        adoptPassword = "";
        adoptImportConfig = false;
    }

    async function confirmAdopt() {
        if (!adoptingDevice) return;

        isAdopting = true;
        try {
            const body = { host: adoptingDevice.ip };
            if (adoptPassword) body.password = adoptPassword;
            if (adoptImportConfig) body.import_config = true;
            const resp = await ApiClient.send("/api/ssh/adopt", {
                method: "POST",
                body: JSON.stringify(body),
                headers: { "Content-Type": "application/json" },
            });
            if (resp.adopted) {
                addSuccessToast(`Device ${resp.hostname || resp.host} adopted successfully!`);
                adoptingDevice = null;
                showDiscovery = false;
                discoveredDevices = [];
                await loadDevices();
            } else {
                addErrorToast(resp.error || "Adoption failed");
            }
        } catch (err) {
            addErrorToast(err?.data?.error || err?.message || "Adoption failed");
        }
        isAdopting = false;
    }

    async function pushConfig(device) {
        try {
            const resp = await ApiClient.send(`/api/ssh/push-config/${device.id}`, {
                method: "POST",
            });
            addSuccessToast(`Config pushed to ${device.name}: ${resp.message}`);
            await loadDevices();
        } catch (err) {
            addErrorToast(err?.data?.error || err?.message || "Config push failed");
        }
    }

    async function rebootDevice(device) {
        if (!confirm(`Reboot ${device.name}?`)) return;
        try {
            await ApiClient.send(`/api/ssh/device/${device.id}/reboot`, {
                method: "POST",
            });
            addSuccessToast(`${device.name} is rebooting...`);
        } catch (err) {
            addErrorToast(err?.data?.error || "Reboot failed");
        }
    }

    function getStatusClass(status) {
        switch (status) {
            case "healthy": return "label-success";
            case "unhealthy": return "label-danger";
            default: return "label-hint";
        }
    }

    function getConfigStatusClass(status) {
        switch (status) {
            case "applied": return "label-success";
            case "error": return "label-danger";
            case "modified": return "label-warning";
            default: return "label-hint";
        }
    }

    function cancelAdopt() {
        adoptingDevice = null;
        adoptPassword = "";
    }
</script>

<PageWrapper>
    <header class="page-header">
        <nav class="breadcrumbs">
            <div class="breadcrumb-item">SSH Management</div>
            <div class="breadcrumb-item">Devices</div>
        </nav>
        <div class="btns-group">
            <a href="/ssh/leds" class="btn btn-outline" use:link>
                <i class="ri-lightbulb-line" />
                <span class="txt">LEDs</span>
            </a>
            <a href="/ssh/dawn" class="btn btn-outline" use:link>
                <i class="ri-router-line" />
                <span class="txt">DAWN</span>
            </a>
            <a href="/ssh/profiles" class="btn btn-outline" use:link>
                <i class="ri-layout-grid-line" />
                <span class="txt">Profiles</span>
            </a>
            <button
                type="button"
                class="btn btn-outline"
                on:click={() => { showDiscovery = !showDiscovery; }}
            >
                <i class="ri-radar-line" />
                <span class="txt">Discover</span>
            </button>
            <a href="/collections?collection=devices" class="btn btn-outline" use:link>
                <i class="ri-database-2-line" />
                <span class="txt">All Records</span>
            </a>
        </div>
    </header>

    <!-- Discovery Panel -->
    {#if showDiscovery}
        <div class="panel m-b-base discovery-panel">
            <div class="panel-header">
                <h6><i class="ri-radar-line" /> Network Discovery</h6>
            </div>
            <div class="panel-content">
                <div class="grid m-b-sm">
                    <div class="col-lg-8">
                        <div class="form-field">
                            <label>Network Range (CIDR)</label>
                            <input
                                type="text"
                                bind:value={discoveryCidr}
                                placeholder="192.168.1.0/24"
                            />
                        </div>
                    </div>
                    <div class="col-lg-4 flex flex-align-end">
                        <button
                            type="button"
                            class="btn btn-expanded"
                            class:btn-loading={isDiscovering}
                            disabled={isDiscovering}
                            on:click={discover}
                        >
                            <i class="ri-search-line" />
                            <span class="txt">{isDiscovering ? 'Scanning...' : 'Scan Network'}</span>
                        </button>
                    </div>
                </div>

                {#if discoveredDevices.length > 0}
                    <div class="table-wrapper">
                        <table class="table">
                            <thead>
                                <tr>
                                    <th>IP</th>
                                    <th>OpenWrt</th>
                                    <th>Model</th>
                                    <th>Hostname</th>
                                    <th>Version</th>
                                    <th class="col-actions">Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                {#each discoveredDevices as device}
                                    <tr>
                                        <td><code>{device.ip}</code></td>
                                        <td>
                                            {#if device.is_openwrt}
                                                <span class="label label-success">Yes</span>
                                            {:else if device.ssh_open}
                                                <span class="label label-warning">SSH only</span>
                                            {:else}
                                                <span class="label label-hint">No</span>
                                            {/if}
                                        </td>
                                        <td>{device.model || '—'}</td>
                                        <td>{device.hostname || '—'}</td>
                                        <td>{device.version || '—'}</td>
                                        <td class="col-actions">
                                            {#if device.is_openwrt}
                                                <button
                                                    type="button"
                                                    class="btn btn-sm btn-success"
                                                    on:click={() => adoptDevice(device)}
                                                >
                                                    <i class="ri-add-circle-line" />
                                                    <span class="txt">Adopt</span>
                                                </button>
                                            {/if}
                                        </td>
                                    </tr>
                                {/each}
                            </tbody>
                        </table>
                    </div>
                {/if}
            </div>
        </div>
    {/if}

    <!-- Adopt Modal -->
    {#if adoptingDevice}
        <div class="overlay active" on:click|self={cancelAdopt}>
            <div class="overlay-panel adopt-modal">
                <div class="panel-header">
                    <h5>
                        <i class="ri-router-line" />
                        Adopt Device
                    </h5>
                    <button type="button" class="btn btn-sm btn-circle btn-transparent" on:click={cancelAdopt}>
                        <i class="ri-close-line" />
                    </button>
                </div>
                <div class="panel-content">
                    <div class="device-info-grid m-b-base">
                        <div class="info-item">
                            <span class="info-label">IP Address</span>
                            <span class="info-value"><code>{adoptingDevice.ip}</code></span>
                        </div>
                        {#if adoptingDevice.model}
                            <div class="info-item">
                                <span class="info-label">Model</span>
                                <span class="info-value">{adoptingDevice.model}</span>
                            </div>
                        {/if}
                        {#if adoptingDevice.hostname}
                            <div class="info-item">
                                <span class="info-label">Hostname</span>
                                <span class="info-value">{adoptingDevice.hostname}</span>
                            </div>
                        {/if}
                        {#if adoptingDevice.version}
                            <div class="info-item">
                                <span class="info-label">Version</span>
                                <span class="info-value">{adoptingDevice.version}</span>
                            </div>
                        {/if}
                    </div>

                    <div class="form-field">
                        <label>SSH Password <small class="txt-hint">(leave empty for default OpenWrt — root with no password)</small></label>
                        <input
                            type="password"
                            bind:value={adoptPassword}
                            placeholder="Empty = no password"
                        />
                    </div>

                    <div class="form-field m-t-sm">
                        <label class="toggle-label">
                            <input type="checkbox" bind:checked={adoptImportConfig} />
                            <span>Import existing configuration</span>
                        </label>
                        <small class="txt-hint">Read the current WiFi, radio, LED and DAWN config from the device and import it into OpenSOHO. No config push will be performed.</small>
                    </div>

                    {#if adoptImportConfig}
                        <div class="alert alert-warning m-t-sm">
                            <i class="ri-information-line" />
                            <span>The device will keep its current configuration. OpenSOHO will only read and store it.</span>
                        </div>
                    {:else}
                        <div class="alert alert-info m-t-sm">
                            <i class="ri-information-line" />
                            <span>The server will inject its SSH public key and push the OpenSOHO configuration to the device.</span>
                        </div>
                    {/if}
                </div>
                <div class="panel-footer">
                    <button type="button" class="btn btn-transparent" on:click={cancelAdopt}>
                        Cancel
                    </button>
                    <button
                        type="button"
                        class="btn btn-expanded btn-success"
                        class:btn-loading={isAdopting}
                        disabled={isAdopting}
                        on:click={confirmAdopt}
                    >
                        <i class="ri-check-line" />
                        <span class="txt">{isAdopting ? 'Adopting...' : 'Adopt Device'}</span>
                    </button>
                </div>
            </div>
        </div>
    {/if}

    <!-- Devices Table -->
    <div class="wrapper">
        {#if isLoading}
            <div class="loader" />
        {:else if devices.length === 0}
            <div class="panel txt-center p-base">
                <div class="content txt-xl m-b-sm">
                    <i class="ri-router-line" style="font-size: 48px; color: var(--txtHintColor);" />
                </div>
                <p class="txt-hint m-b-base">No devices found. Use <strong>Discover</strong> to scan your network.</p>
                <button type="button" class="btn btn-expanded" on:click={() => { showDiscovery = true; }}>
                    <i class="ri-radar-line" />
                    <span class="txt">Start Discovery</span>
                </button>
            </div>
        {:else}
            <div class="table-wrapper">
                <table class="table" id="devices-table">
                    <thead>
                        <tr>
                            <th class="col-indicator" />
                            <th>Name</th>
                            <th>Model</th>
                            <th>IP Address</th>
                            <th>MAC</th>
                            <th>Health</th>
                            <th>Config</th>
                            <th class="col-actions">Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        {#each devices as device (device.id)}
                            <tr>
                                <td class="col-indicator">
                                    <span
                                        class="indicator"
                                        class:indicator-success={device.health_status === 'healthy'}
                                        class:indicator-danger={device.health_status === 'unhealthy'}
                                    />
                                </td>
                                <td class="txt-bold">
                                    <a href="/ssh/device/{device.id}" use:link>{device.name || 'Unnamed'}</a>
                                </td>
                                <td class="txt-hint">{device.model || '—'}</td>
                                <td><code>{device.ip_address || '—'}</code></td>
                                <td><code class="txt-sm">{device.mac_address || '—'}</code></td>
                                <td>
                                    <span class="label {getStatusClass(device.health_status)}">
                                        {device.health_status || 'unknown'}
                                    </span>
                                </td>
                                <td>
                                    <span class="label {getConfigStatusClass(device.config_status)}">
                                        {device.config_status || 'pending'}
                                    </span>
                                </td>
                                <td class="col-actions">
                                    <div class="inline-flex flex-gap-5">
                                        <button
                                            type="button"
                                            class="btn btn-sm"
                                            title="Push Config"
                                            on:click={() => pushConfig(device)}
                                        >
                                            <i class="ri-upload-cloud-line" />
                                        </button>
                                        <a
                                            href="/ssh/device/{device.id}"
                                            class="btn btn-sm btn-outline"
                                            title="Details"
                                            use:link
                                        >
                                            <i class="ri-eye-line" />
                                        </a>
                                        <button
                                            type="button"
                                            class="btn btn-sm btn-danger btn-outline"
                                            title="Reboot"
                                            on:click={() => rebootDevice(device)}
                                        >
                                            <i class="ri-restart-line" />
                                        </button>
                                    </div>
                                </td>
                            </tr>
                        {/each}
                    </tbody>
                </table>
            </div>
        {/if}
    </div>

    <!-- SSH Key Info -->
    {#if publicKey}
        <div class="panel m-t-base ssh-key-panel">
            <div class="panel-header">
                <h6><i class="ri-key-line" /> Server SSH Public Key</h6>
            </div>
            <div class="panel-content">
                <p class="txt-hint txt-sm m-b-xs">This key is automatically injected into devices during adoption. You can also copy it for manual setup:</p>
                <div class="key-display">
                    <code>{publicKey}</code>
                    <button
                        type="button"
                        class="btn btn-sm btn-transparent"
                        title="Copy to clipboard"
                        on:click={() => { navigator.clipboard.writeText(publicKey); addSuccessToast('Key copied!'); }}
                    >
                        <i class="ri-clipboard-line" />
                    </button>
                </div>
            </div>
        </div>
    {/if}
</PageWrapper>

<style>
    .discovery-panel {
        border: 2px solid var(--infoAltColor);
    }
    .discovery-panel .panel-header {
        padding: 12px 20px;
        background: var(--infoAltColor);
        border-bottom: 1px solid var(--baseAlt1Color);
    }
    .discovery-panel .panel-header h6 {
        margin: 0;
        display: flex;
        align-items: center;
        gap: 8px;
    }
    .discovery-panel .panel-content {
        padding: 20px;
    }

    .col-actions {
        width: 1%;
        white-space: nowrap;
    }
    .col-indicator {
        width: 1%;
        padding-right: 0 !important;
    }

    .indicator {
        display: inline-block;
        width: 8px;
        height: 8px;
        border-radius: 50%;
        background: var(--baseAlt3Color);
    }
    .indicator-success { background: var(--successColor); }
    .indicator-danger { background: var(--dangerColor); }

    .label {
        display: inline-block;
        padding: 2px 8px;
        border-radius: 30px;
        font-size: var(--xsFontSize);
        font-weight: 600;
        text-transform: capitalize;
    }
    .label-success { background: var(--successAltColor); color: #1a6b4a; }
    .label-danger  { background: var(--dangerAltColor);  color: #a82a42; }
    .label-warning { background: var(--warningAltColor); color: #8a5a2a; }
    .label-hint    { background: var(--baseAlt1Color);   color: var(--txtHintColor); }

    .inline-flex {
        display: inline-flex;
        align-items: center;
    }

    /* Overlay/Modal */
    .overlay {
        position: fixed;
        inset: 0;
        z-index: 1000;
        display: flex;
        align-items: center;
        justify-content: center;
        background: var(--overlayColor);
    }
    .overlay-panel {
        background: var(--baseColor);
        border-radius: var(--lgRadius);
        box-shadow: 0 8px 40px var(--shadowColor);
        max-width: 520px;
        width: 100%;
        max-height: 90vh;
        overflow-y: auto;
    }
    .overlay-panel .panel-header {
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding: 16px 20px;
        border-bottom: 1px solid var(--baseAlt1Color);
    }
    .overlay-panel .panel-header h5 {
        display: flex;
        align-items: center;
        gap: 8px;
        margin: 0;
    }
    .overlay-panel .panel-content {
        padding: 20px;
    }
    .overlay-panel .panel-footer {
        display: flex;
        justify-content: flex-end;
        gap: 8px;
        padding: 12px 20px;
        border-top: 1px solid var(--baseAlt1Color);
    }

    .device-info-grid {
        display: grid;
        grid-template-columns: 1fr 1fr;
        gap: 12px;
    }
    .info-item {
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .info-label {
        font-size: var(--xsFontSize);
        color: var(--txtHintColor);
        text-transform: uppercase;
        letter-spacing: 0.05em;
    }
    .info-value {
        font-weight: 600;
    }

    .alert {
        display: flex;
        align-items: flex-start;
        gap: 8px;
        padding: 12px;
        border-radius: var(--baseRadius);
        font-size: var(--smFontSize);
    }
    .alert-warning {
        background: var(--warningAltColor);
        color: #8a5a2a;
    }
    .toggle-label {
        display: flex;
        align-items: center;
        gap: 8px;
        cursor: pointer;
        font-weight: 600;
    }

    .ssh-key-panel .panel-header {
        padding: 12px 20px;
        border-bottom: 1px solid var(--baseAlt1Color);
    }
    .ssh-key-panel .panel-header h6 {
        margin: 0;
        display: flex;
        align-items: center;
        gap: 8px;
    }
    .ssh-key-panel .panel-content {
        padding: 16px 20px;
    }
    .key-display {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 8px 12px;
        background: var(--bodyColor);
        border-radius: var(--baseRadius);
        overflow-x: auto;
    }
    .key-display code {
        flex: 1;
        font-size: var(--xsFontSize);
        word-break: break-all;
    }
</style>

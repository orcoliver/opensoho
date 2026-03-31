<script>
    import { pageTitle } from "@/stores/app";
    import { addSuccessToast, addErrorToast } from "@/stores/toasts";
    import ApiClient from "@/utils/ApiClient";
    import PageWrapper from "@/components/base/PageWrapper.svelte";
    import { link } from "svelte-spa-router";

    $pageTitle = "LED Configuration";

    const TRIGGERS = [
        "none", "default-on", "heartbeat", "timer", "netdev",
        "phy0rx", "phy0tx", "phy0assoc", "phy0radio", "phy0tpt",
        "phy1rx", "phy1tx", "phy1assoc", "phy1radio", "phy1tpt",
    ];

    const TRIGGER_COLORS = {
        "none": "label-hint",
        "default-on": "label-success",
        "heartbeat": "label-warning",
        "timer": "label-warning",
        "netdev": "label-info",
    };

    let leds = [];
    let isLoading = true;
    let showPanel = false;
    let editingLed = null;
    let isSaving = false;

    const emptyForm = () => ({
        name: "", led_name: "", trigger: "none",
        dev: "", mode: [], delayon: 500, delayoff: 500,
    });
    let form = emptyForm();

    load();

    async function load() {
        isLoading = true;
        try {
            leds = await ApiClient.collection("leds").getFullList({ sort: "name" });
        } catch (err) {
            ApiClient.error(err);
        }
        isLoading = false;
    }

    function openNew() {
        editingLed = null;
        form = emptyForm();
        showPanel = true;
    }

    function openEdit(led) {
        editingLed = led;
        form = {
            name: led.name || "",
            led_name: led.led_name || "",
            trigger: led.trigger || "none",
            dev: led.dev || "",
            mode: led.mode || [],
            delayon: led.delayon || 500,
            delayoff: led.delayoff || 500,
        };
        showPanel = true;
    }

    async function save() {
        isSaving = true;
        try {
            const data = {
                name: form.name,
                led_name: form.led_name,
                trigger: form.trigger,
                dev: form.trigger === "netdev" ? form.dev : "",
                mode: form.trigger === "netdev" ? form.mode : [],
                delayon: form.trigger === "timer" ? form.delayon : null,
                delayoff: form.trigger === "timer" ? form.delayoff : null,
            };
            if (editingLed) {
                await ApiClient.collection("leds").update(editingLed.id, data);
                addSuccessToast("LED updated");
            } else {
                await ApiClient.collection("leds").create(data);
                addSuccessToast("LED created");
            }
            showPanel = false;
            await load();
        } catch (err) {
            addErrorToast(err?.data?.message || "Save failed");
        }
        isSaving = false;
    }

    async function deleteLed(led) {
        if (!confirm(`Delete LED "${led.name}"?`)) return;
        try {
            await ApiClient.collection("leds").delete(led.id);
            addSuccessToast("LED deleted");
            await load();
        } catch (err) {
            addErrorToast(err?.data?.message || "Delete failed");
        }
    }

    function triggerClass(trigger) {
        return TRIGGER_COLORS[trigger] || "label-hint";
    }

    function toggleMode(m) {
        if (form.mode.includes(m)) {
            form.mode = form.mode.filter(x => x !== m);
        } else {
            form.mode = [...form.mode, m];
        }
    }
</script>

<PageWrapper>
    <header class="page-header">
        <nav class="breadcrumbs">
            <a href="/ssh/devices" class="breadcrumb-item" use:link>SSH Management</a>
            <div class="breadcrumb-item">LEDs</div>
        </nav>
        <div class="btns-group">
            <button type="button" class="btn btn-expanded" on:click={openNew}>
                <i class="ri-add-line" />
                <span class="txt">New LED</span>
            </button>
        </div>
    </header>

    <div class="wrapper">
        {#if isLoading}
            <div class="loader" />
        {:else if leds.length === 0}
            <div class="panel txt-center p-base">
                <i class="ri-lightbulb-line" style="font-size:48px;color:var(--txtHintColor)" />
                <p class="txt-hint m-t-sm">No LEDs configured. Click <strong>New LED</strong> to add one.</p>
            </div>
        {:else}
            <div class="table-wrapper">
                <table class="table">
                    <thead>
                        <tr>
                            <th>Name</th>
                            <th>Sysfs path</th>
                            <th>Trigger</th>
                            <th>Details</th>
                            <th class="col-actions">Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        {#each leds as led (led.id)}
                            <tr>
                                <td class="txt-bold">{led.name || "—"}</td>
                                <td><code class="txt-sm">{led.led_name || "—"}</code></td>
                                <td>
                                    <span class="label {triggerClass(led.trigger)}">
                                        {led.trigger || "none"}
                                    </span>
                                </td>
                                <td class="txt-hint txt-sm">
                                    {#if led.trigger === "netdev"}
                                        dev: {led.dev || "—"} &nbsp;|&nbsp; mode: {(led.mode || []).join(" ") || "—"}
                                    {:else if led.trigger === "timer"}
                                        on: {led.delayon}ms &nbsp;|&nbsp; off: {led.delayoff}ms
                                    {:else}
                                        —
                                    {/if}
                                </td>
                                <td class="col-actions">
                                    <div class="inline-flex flex-gap-5">
                                        <button type="button" class="btn btn-sm btn-outline" on:click={() => openEdit(led)}>
                                            <i class="ri-edit-line" />
                                        </button>
                                        <button type="button" class="btn btn-sm btn-danger btn-outline" on:click={() => deleteLed(led)}>
                                            <i class="ri-delete-bin-line" />
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
</PageWrapper>

<!-- Side panel -->
{#if showPanel}
    <div class="overlay active" on:click|self={() => { showPanel = false; }}>
        <div class="overlay-panel">
            <div class="panel-header">
                <h5><i class="ri-lightbulb-line" /> {editingLed ? "Edit LED" : "New LED"}</h5>
                <button type="button" class="btn btn-sm btn-circle btn-transparent" on:click={() => { showPanel = false; }}>
                    <i class="ri-close-line" />
                </button>
            </div>
            <div class="panel-content">
                <div class="form-field m-b-sm">
                    <label>Name <small class="txt-hint">(human readable)</small></label>
                    <input type="text" bind:value={form.name} placeholder="WAN" />
                </div>
                <div class="form-field m-b-sm">
                    <label>Sysfs path <small class="txt-hint">(from /sys/class/leds/)</small></label>
                    <input type="text" bind:value={form.led_name} placeholder="green:status" />
                </div>
                <div class="form-field m-b-sm">
                    <label>Trigger</label>
                    <select bind:value={form.trigger}>
                        {#each TRIGGERS as t}
                            <option value={t}>{t}</option>
                        {/each}
                    </select>
                </div>

                {#if form.trigger === "netdev"}
                    <div class="form-field m-b-sm">
                        <label>Network interface</label>
                        <input type="text" bind:value={form.dev} placeholder="eth0" />
                    </div>
                    <div class="form-field m-b-sm">
                        <label>Mode</label>
                        <div class="inline-flex flex-gap-10">
                            {#each ["link", "tx", "rx"] as m}
                                <label class="checkbox-label">
                                    <input
                                        type="checkbox"
                                        checked={form.mode.includes(m)}
                                        on:change={() => toggleMode(m)}
                                    />
                                    {m}
                                </label>
                            {/each}
                        </div>
                    </div>
                {/if}

                {#if form.trigger === "timer"}
                    <div class="grid">
                        <div class="col-6">
                            <div class="form-field m-b-sm">
                                <label>Delay on (ms)</label>
                                <input type="number" min="0" bind:value={form.delayon} />
                            </div>
                        </div>
                        <div class="col-6">
                            <div class="form-field m-b-sm">
                                <label>Delay off (ms)</label>
                                <input type="number" min="0" bind:value={form.delayoff} />
                            </div>
                        </div>
                    </div>
                {/if}
            </div>
            <div class="panel-footer">
                <button type="button" class="btn btn-transparent" on:click={() => { showPanel = false; }}>Cancel</button>
                <button
                    type="button"
                    class="btn btn-expanded btn-success"
                    class:btn-loading={isSaving}
                    disabled={isSaving}
                    on:click={save}
                >
                    <i class="ri-check-line" />
                    <span class="txt">Save</span>
                </button>
            </div>
        </div>
    </div>
{/if}

<style>
    .col-actions { width: 1%; white-space: nowrap; }
    .inline-flex { display: inline-flex; align-items: center; }
    .flex-gap-5 { gap: 5px; }
    .flex-gap-10 { gap: 10px; }
    .label {
        display: inline-block; padding: 2px 8px; border-radius: 30px;
        font-size: var(--xsFontSize); font-weight: 600; text-transform: capitalize;
    }
    .label-success { background: var(--successAltColor); color: #1a6b4a; }
    .label-warning { background: var(--warningAltColor); color: #8a5a2a; }
    .label-info    { background: var(--infoAltColor);    color: #2d6bb0; }
    .label-hint    { background: var(--baseAlt1Color);   color: var(--txtHintColor); }
    .overlay {
        position: fixed; inset: 0; z-index: 1000;
        display: flex; align-items: center; justify-content: center;
        background: var(--overlayColor);
    }
    .overlay-panel {
        background: var(--baseColor); border-radius: var(--lgRadius);
        box-shadow: 0 8px 40px var(--shadowColor);
        max-width: 480px; width: 100%; max-height: 90vh; overflow-y: auto;
    }
    .overlay-panel .panel-header {
        display: flex; align-items: center; justify-content: space-between;
        padding: 16px 20px; border-bottom: 1px solid var(--baseAlt1Color);
    }
    .overlay-panel .panel-header h5 { display: flex; align-items: center; gap: 8px; margin: 0; }
    .overlay-panel .panel-content { padding: 20px; }
    .overlay-panel .panel-footer {
        display: flex; justify-content: flex-end; gap: 8px;
        padding: 12px 20px; border-top: 1px solid var(--baseAlt1Color);
    }
    .checkbox-label { display: flex; align-items: center; gap: 4px; cursor: pointer; }
</style>

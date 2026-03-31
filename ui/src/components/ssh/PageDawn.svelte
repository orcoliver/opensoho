<script>
    import { pageTitle } from "@/stores/app";
    import { addSuccessToast, addErrorToast } from "@/stores/toasts";
    import ApiClient from "@/utils/ApiClient";
    import PageWrapper from "@/components/base/PageWrapper.svelte";
    import { link } from "svelte-spa-router";

    $pageTitle = "DAWN Configuration";

    const DEFAULTS = {
        kicking: true, set_hostapd_nr: true, rrm_mode: "pat",
        initial_score: 200, ht_support: 4, vht_support: 8, he_support: 16,
        rssi: 10, rssi_val: -60, low_rssi: -5, low_rssi_val: -80,
        freq_5: 100, chan_util: -5, rssi_weight: 1, rssi_center: 2,
        update_client: 10, remove_client: 15, remove_probe: 30,
        update_hostapd: 10, update_tcp_con: 30, update_chan_util: 5,
        update_beacon_reports: 20,
        kicking_threshold: -65, min_probe_count: 2, bandwidth_threshold: 6,
        use_station_count: true, max_station_diff: 1, min_number_to_kick: 3,
        chan_util_avg_period: 3, min_kick_count: 1,
    };

    let record = null;
    let form = { ...DEFAULTS };
    let isLoading = true;
    let isSaving = false;
    let openSection = "global";

    load();

    async function load() {
        isLoading = true;
        try {
            const list = await ApiClient.collection("dawn").getList(1, 1);
            if (list.items.length > 0) {
                record = list.items[0];
                form = { ...DEFAULTS, ...record };
            }
        } catch (_) {}
        isLoading = false;
    }

    async function save() {
        isSaving = true;
        try {
            if (record) {
                record = await ApiClient.collection("dawn").update(record.id, form);
            } else {
                record = await ApiClient.collection("dawn").create(form);
            }
            addSuccessToast("DAWN configuration saved");
        } catch (err) {
            addErrorToast(err?.data?.message || "Save failed");
        }
        isSaving = false;
    }

    function resetDefaults() {
        form = { ...DEFAULTS };
    }

    function toggle(section) {
        openSection = openSection === section ? null : section;
    }
</script>

<PageWrapper>
    <header class="page-header">
        <nav class="breadcrumbs">
            <a href="/ssh/devices" class="breadcrumb-item" use:link>SSH Management</a>
            <div class="breadcrumb-item">DAWN</div>
        </nav>
        <div class="btns-group">
            <button type="button" class="btn btn-outline" on:click={resetDefaults}>
                <i class="ri-refresh-line" />
                <span class="txt">Reset to defaults</span>
            </button>
            <button
                type="button"
                class="btn btn-expanded"
                class:btn-loading={isSaving}
                disabled={isSaving}
                on:click={save}
            >
                <i class="ri-save-line" />
                <span class="txt">Save</span>
            </button>
        </div>
    </header>

    {#if isLoading}
        <div class="loader" />
    {:else}
        <div class="alert alert-info m-b-base">
            <i class="ri-information-line" />
            <span>DAWN (Dynamic AirTime Network) handles automatic client steering across APs. This configuration is applied to all devices that have DAWN installed.</span>
        </div>

        <!-- Global -->
        <div class="accordion m-b-sm" class:active={openSection === "global"}>
            <button class="accordion-header" type="button" on:click={() => toggle("global")}>
                <i class="ri-global-line" /> Global
                <i class="ri-arrow-down-s-line accordion-arrow" />
            </button>
            {#if openSection === "global"}
                <div class="accordion-content">
                    <div class="grid">
                        <div class="col-lg-4">
                            <label class="form-field">
                                <input type="checkbox" bind:checked={form.kicking} />
                                <span>Kicking enabled</span>
                                <small class="txt-hint">Allow DAWN to kick clients to better APs</small>
                            </label>
                        </div>
                        <div class="col-lg-4">
                            <label class="form-field">
                                <input type="checkbox" bind:checked={form.set_hostapd_nr} />
                                <span>Set hostapd neighbor report</span>
                            </label>
                        </div>
                        <div class="col-lg-4">
                            <div class="form-field">
                                <label>RRM Mode</label>
                                <select bind:value={form.rrm_mode}>
                                    <option value="pat">pat — proactive, threshold-based</option>
                                    <option value="all">all — kick all eligible clients</option>
                                    <option value="off">off — disabled</option>
                                </select>
                            </div>
                        </div>
                    </div>
                </div>
            {/if}
        </div>

        <!-- Metric -->
        <div class="accordion m-b-sm" class:active={openSection === "metric"}>
            <button class="accordion-header" type="button" on:click={() => toggle("metric")}>
                <i class="ri-bar-chart-line" /> Metric (scoring)
                <i class="ri-arrow-down-s-line accordion-arrow" />
            </button>
            {#if openSection === "metric"}
                <div class="accordion-content">
                    <div class="grid">
                        {#each [
                            ["initial_score", "Initial score", "Base score for each client"],
                            ["ht_support", "HT support bonus", "Score bonus for 802.11n"],
                            ["vht_support", "VHT support bonus", "Score bonus for 802.11ac"],
                            ["he_support", "HE support bonus", "Score bonus for 802.11ax (Wi-Fi 6)"],
                            ["rssi", "RSSI bonus", "Score bonus per RSSI unit above threshold"],
                            ["rssi_val", "RSSI threshold", "dBm threshold for RSSI bonus (negative)"],
                            ["low_rssi", "Low RSSI penalty", "Score penalty for weak signal"],
                            ["low_rssi_val", "Low RSSI threshold", "dBm threshold for penalty (negative)"],
                            ["freq_5", "5 GHz bonus", "Score bonus for 5 GHz band"],
                            ["chan_util", "Channel utilization penalty", "Score penalty per % channel utilization"],
                            ["rssi_weight", "RSSI weight", "Weight multiplier for RSSI score"],
                            ["rssi_center", "RSSI center", "Center value for RSSI scoring"],
                        ] as [field, label, hint]}
                            <div class="col-lg-3">
                                <div class="form-field">
                                    <label>{label}</label>
                                    <input type="number" bind:value={form[field]} />
                                    <small class="txt-hint">{hint}</small>
                                </div>
                            </div>
                        {/each}
                    </div>
                </div>
            {/if}
        </div>

        <!-- Times -->
        <div class="accordion m-b-sm" class:active={openSection === "times"}>
            <button class="accordion-header" type="button" on:click={() => toggle("times")}>
                <i class="ri-timer-line" /> Times (seconds)
                <i class="ri-arrow-down-s-line accordion-arrow" />
            </button>
            {#if openSection === "times"}
                <div class="accordion-content">
                    <div class="grid">
                        {#each [
                            ["update_client", "Update client interval"],
                            ["remove_client", "Remove client timeout"],
                            ["remove_probe", "Remove probe timeout"],
                            ["update_hostapd", "Update hostapd interval"],
                            ["update_tcp_con", "Update TCP connections interval"],
                            ["update_chan_util", "Update channel utilization interval"],
                            ["update_beacon_reports", "Update beacon reports interval"],
                        ] as [field, label]}
                            <div class="col-lg-3">
                                <div class="form-field">
                                    <label>{label}</label>
                                    <input type="number" min="1" bind:value={form[field]} />
                                </div>
                            </div>
                        {/each}
                    </div>
                </div>
            {/if}
        </div>

        <!-- Behaviour -->
        <div class="accordion m-b-sm" class:active={openSection === "behaviour"}>
            <button class="accordion-header" type="button" on:click={() => toggle("behaviour")}>
                <i class="ri-settings-3-line" /> Behaviour
                <i class="ri-arrow-down-s-line accordion-arrow" />
            </button>
            {#if openSection === "behaviour"}
                <div class="accordion-content">
                    <div class="grid">
                        <div class="col-lg-3">
                            <div class="form-field">
                                <label>Kicking threshold (dBm)</label>
                                <input type="number" bind:value={form.kicking_threshold} />
                                <small class="txt-hint">RSSI below this triggers kick</small>
                            </div>
                        </div>
                        <div class="col-lg-3">
                            <div class="form-field">
                                <label>Min probe count</label>
                                <input type="number" min="1" bind:value={form.min_probe_count} />
                            </div>
                        </div>
                        <div class="col-lg-3">
                            <div class="form-field">
                                <label>Bandwidth threshold (Mbps)</label>
                                <input type="number" min="0" bind:value={form.bandwidth_threshold} />
                            </div>
                        </div>
                        <div class="col-lg-3">
                            <label class="form-field">
                                <input type="checkbox" bind:checked={form.use_station_count} />
                                <span>Use station count</span>
                            </label>
                        </div>
                        <div class="col-lg-3">
                            <div class="form-field">
                                <label>Max station diff</label>
                                <input type="number" min="0" bind:value={form.max_station_diff} />
                            </div>
                        </div>
                        <div class="col-lg-3">
                            <div class="form-field">
                                <label>Min number to kick</label>
                                <input type="number" min="1" bind:value={form.min_number_to_kick} />
                            </div>
                        </div>
                        <div class="col-lg-3">
                            <div class="form-field">
                                <label>Chan util avg period</label>
                                <input type="number" min="1" bind:value={form.chan_util_avg_period} />
                            </div>
                        </div>
                        <div class="col-lg-3">
                            <div class="form-field">
                                <label>Min kick count</label>
                                <input type="number" min="1" bind:value={form.min_kick_count} />
                            </div>
                        </div>
                    </div>
                </div>
            {/if}
        </div>
    {/if}
</PageWrapper>

<style>
    .accordion {
        border: 1px solid var(--baseAlt1Color);
        border-radius: var(--baseRadius);
        overflow: hidden;
    }
    .accordion-header {
        width: 100%;
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 12px 16px;
        background: var(--baseAlt1Color);
        border: none;
        cursor: pointer;
        font-weight: 600;
        text-align: left;
        color: var(--txtPrimaryColor);
    }
    .accordion-arrow { margin-left: auto; transition: transform 0.2s; }
    .accordion.active .accordion-arrow { transform: rotate(180deg); }
    .accordion-content { padding: 16px; }
    .alert {
        display: flex;
        align-items: flex-start;
        gap: 8px;
        padding: 12px 16px;
        border-radius: var(--baseRadius);
        font-size: var(--smFontSize);
    }
    .alert-info { background: var(--infoAltColor); color: #2d6bb0; }
</style>

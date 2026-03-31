<script>
    import { pageTitle } from "@/stores/app";
    import { addSuccessToast, addErrorToast } from "@/stores/toasts";
    import ApiClient from "@/utils/ApiClient";
    import PageWrapper from "@/components/base/PageWrapper.svelte";
    import { link } from "svelte-spa-router";

    $pageTitle = "Device Profiles";

    const MODES = ["dumb_ap", "router", "mesh_node"];
    const LAN_PROTOS = ["dhcp", "static", "none"];

    let profiles = [];
    let isLoading = true;
    let showPanel = false;
    let editingProfile = null;
    let isSaving = false;

    const emptyForm = () => ({
        name: "",
        mode: "dumb_ap",
        lan_proto: "dhcp",
        lan_cidr: "",
        disable_firewall: false,
        disable_dnsmasq: false,
        disable_odhcpd: false,
        igmp_snooping: false,
        bridge_vlan_filtering: false,
        stp: false,
        extra_networks: [],
    });
    let form = emptyForm();

    load();

    async function load() {
        isLoading = true;
        try {
            profiles = await ApiClient.collection("device_profile").getFullList({ sort: "name" });
        } catch (err) {
            ApiClient.error(err);
        }
        isLoading = false;
    }

    function openNew() {
        editingProfile = null;
        form = emptyForm();
        showPanel = true;
    }

    function openEdit(profile) {
        editingProfile = profile;
        let extras = [];
        try {
            extras = typeof profile.extra_networks === "string"
                ? JSON.parse(profile.extra_networks)
                : (profile.extra_networks || []);
        } catch {}
        form = {
            name: profile.name || "",
            mode: profile.mode || "dumb_ap",
            lan_proto: profile.lan_proto || "dhcp",
            lan_cidr: profile.lan_cidr || "",
            disable_firewall: !!profile.disable_firewall,
            disable_dnsmasq: !!profile.disable_dnsmasq,
            disable_odhcpd: !!profile.disable_odhcpd,
            igmp_snooping: !!profile.igmp_snooping,
            bridge_vlan_filtering: !!profile.bridge_vlan_filtering,
            stp: !!profile.stp,
            extra_networks: extras.map(en => ({
                name: en.name || "",
                device: en.device || "",
                proto: en.proto || "none",
                bridge_ports: Array.isArray(en.bridge_ports) ? en.bridge_ports.join(", ") : (en.bridge_ports || ""),
            })),
        };
        showPanel = true;
    }

    function addExtraNetwork() {
        form.extra_networks = [...form.extra_networks, { name: "", device: "", proto: "none", bridge_ports: "" }];
    }

    function removeExtraNetwork(i) {
        form.extra_networks = form.extra_networks.filter((_, idx) => idx !== i);
    }

    async function save() {
        isSaving = true;
        try {
            const extras = form.extra_networks
                .filter(en => en.name && en.device)
                .map(en => ({
                    name: en.name,
                    device: en.device,
                    proto: en.proto,
                    bridge_ports: en.bridge_ports
                        ? en.bridge_ports.split(",").map(s => s.trim()).filter(Boolean)
                        : [],
                }));

            const data = {
                name: form.name,
                mode: form.mode,
                lan_proto: form.lan_proto,
                lan_cidr: form.lan_proto === "static" ? form.lan_cidr : "",
                disable_firewall: form.disable_firewall,
                disable_dnsmasq: form.disable_dnsmasq,
                disable_odhcpd: form.disable_odhcpd,
                igmp_snooping: form.igmp_snooping,
                bridge_vlan_filtering: form.bridge_vlan_filtering,
                stp: form.stp,
                extra_networks: extras,
            };

            if (editingProfile) {
                await ApiClient.collection("device_profile").update(editingProfile.id, data);
                addSuccessToast("Profile updated");
            } else {
                await ApiClient.collection("device_profile").create(data);
                addSuccessToast("Profile created");
            }
            showPanel = false;
            await load();
        } catch (err) {
            addErrorToast(err?.data?.message || "Save failed");
        }
        isSaving = false;
    }

    async function deleteProfile(profile) {
        if (!confirm(`Delete profile "${profile.name}"? Make sure no devices are using it.`)) return;
        try {
            await ApiClient.collection("device_profile").delete(profile.id);
            addSuccessToast("Profile deleted");
            await load();
        } catch (err) {
            addErrorToast(err?.data?.message || "Delete failed");
        }
    }

    function modeSummary(profile) {
        const parts = [];
        if (profile.disable_firewall) parts.push("firewall disabled");
        if (profile.disable_dnsmasq) parts.push("dnsmasq disabled");
        if (profile.disable_odhcpd) parts.push("odhcpd disabled");
        if (profile.igmp_snooping) parts.push("IGMP snooping");
        parts.push(`LAN: ${profile.lan_proto}`);
        return parts.join(" · ");
    }

    function modeClass(mode) {
        if (mode === "dumb_ap") return "label-info";
        if (mode === "router") return "label-success";
        return "label-hint";
    }
</script>

<PageWrapper>
    <header class="page-header">
        <nav class="breadcrumbs">
            <a href="/ssh/devices" class="breadcrumb-item" use:link>SSH Management</a>
            <div class="breadcrumb-item">Device Profiles</div>
        </nav>
        <div class="btns-group">
            <button type="button" class="btn btn-expanded" on:click={openNew}>
                <i class="ri-add-line" />
                <span class="txt">New Profile</span>
            </button>
        </div>
    </header>

    <div class="wrapper">
        {#if isLoading}
            <div class="loader" />
        {:else if profiles.length === 0}
            <div class="panel txt-center p-base">
                <i class="ri-layout-grid-line" style="font-size:48px;color:var(--txtHintColor)" />
                <p class="txt-hint m-t-sm">No profiles configured. Click <strong>New Profile</strong> to add one.</p>
            </div>
        {:else}
            <div class="table-wrapper">
                <table class="table">
                    <thead>
                        <tr>
                            <th>Name</th>
                            <th>Mode</th>
                            <th>Summary</th>
                            <th class="col-actions">Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        {#each profiles as profile (profile.id)}
                            <tr>
                                <td class="txt-bold">{profile.name}</td>
                                <td>
                                    <span class="label {modeClass(profile.mode)}">{profile.mode}</span>
                                </td>
                                <td class="txt-hint txt-sm">{modeSummary(profile)}</td>
                                <td class="col-actions">
                                    <div class="inline-flex flex-gap-5">
                                        <button type="button" class="btn btn-sm btn-outline" on:click={() => openEdit(profile)}>
                                            <i class="ri-edit-line" />
                                        </button>
                                        <button type="button" class="btn btn-sm btn-danger btn-outline" on:click={() => deleteProfile(profile)}>
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
                <h5><i class="ri-layout-grid-line" /> {editingProfile ? "Edit Profile" : "New Profile"}</h5>
                <button type="button" class="btn btn-sm btn-circle btn-transparent" on:click={() => { showPanel = false; }}>
                    <i class="ri-close-line" />
                </button>
            </div>
            <div class="panel-content">
                <div class="form-field m-b-sm">
                    <label>Name</label>
                    <input type="text" bind:value={form.name} placeholder="dumb_ap" />
                </div>
                <div class="grid">
                    <div class="col-6">
                        <div class="form-field m-b-sm">
                            <label>Mode</label>
                            <select bind:value={form.mode}>
                                {#each MODES as m}
                                    <option value={m}>{m}</option>
                                {/each}
                            </select>
                        </div>
                    </div>
                    <div class="col-6">
                        <div class="form-field m-b-sm">
                            <label>LAN Protocol</label>
                            <select bind:value={form.lan_proto}>
                                {#each LAN_PROTOS as p}
                                    <option value={p}>{p}</option>
                                {/each}
                            </select>
                        </div>
                    </div>
                </div>

                {#if form.lan_proto === "static"}
                    <div class="form-field m-b-sm">
                        <label>LAN CIDR <small class="txt-hint">(e.g. 192.168.1.1/24)</small></label>
                        <input type="text" bind:value={form.lan_cidr} placeholder="192.168.1.1/24" />
                    </div>
                {/if}

                <div class="section-title m-t-sm m-b-xs txt-hint txt-sm">Services</div>
                <div class="checkboxes-group m-b-sm">
                    <label class="checkbox-label">
                        <input type="checkbox" bind:checked={form.disable_firewall} />
                        Disable firewall
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" bind:checked={form.disable_dnsmasq} />
                        Disable dnsmasq (DHCP/DNS)
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" bind:checked={form.disable_odhcpd} />
                        Disable odhcpd (DHCPv6)
                    </label>
                </div>

                <div class="section-title m-t-sm m-b-xs txt-hint txt-sm">Bridge Options</div>
                <div class="checkboxes-group m-b-sm">
                    <label class="checkbox-label">
                        <input type="checkbox" bind:checked={form.igmp_snooping} />
                        IGMP Snooping
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" bind:checked={form.bridge_vlan_filtering} />
                        Bridge VLAN Filtering (802.1Q)
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" bind:checked={form.stp} />
                        STP
                    </label>
                </div>

                <div class="section-title m-t-sm m-b-xs txt-hint txt-sm">
                    Extra Networks
                    <button type="button" class="btn btn-xs btn-outline m-l-xs" on:click={addExtraNetwork}>
                        <i class="ri-add-line" /> Add
                    </button>
                </div>
                {#each form.extra_networks as en, i}
                    <div class="extra-network-row m-b-xs">
                        <div class="grid">
                            <div class="col-6">
                                <input type="text" bind:value={en.name} placeholder="vlan_invitados" />
                            </div>
                            <div class="col-6">
                                <input type="text" bind:value={en.device} placeholder="br-invitados" />
                            </div>
                        </div>
                        <div class="grid m-t-xs">
                            <div class="col-4">
                                <select bind:value={en.proto}>
                                    {#each LAN_PROTOS as p}
                                        <option value={p}>{p}</option>
                                    {/each}
                                </select>
                            </div>
                            <div class="col-6">
                                <input type="text" bind:value={en.bridge_ports} placeholder="br-lan.10, eth1" />
                            </div>
                            <div class="col-2 txt-right">
                                <button type="button" class="btn btn-sm btn-danger btn-outline" on:click={() => removeExtraNetwork(i)}>
                                    <i class="ri-delete-bin-line" />
                                </button>
                            </div>
                        </div>
                    </div>
                {:else}
                    <p class="txt-hint txt-sm">No extra networks.</p>
                {/each}
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
    .checkboxes-group { display: flex; flex-direction: column; gap: 6px; }
    .checkbox-label { display: inline-flex; align-items: center; gap: 6px; cursor: pointer; }
    .section-title { font-weight: 600; }
    .extra-network-row { border: 1px solid var(--baseAlt2Color); border-radius: 4px; padding: 8px; }
    .m-l-xs { margin-left: 6px; }
    .m-t-xs { margin-top: 4px; }
</style>

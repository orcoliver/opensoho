# OpenSOHO — Plan de implementación: Gestión centralizada estilo Deco/Omada

## Contexto

El objetivo es que OpenSOHO pueda replicar completamente el script `99_custom_dumb_ap` de forma centralizada, permitiendo configurar un router OpenWRT recién instalado desde cero sin intervención manual. El flujo objetivo es:

```
1. Instalar OpenWRT en el router
2. Conectar a la red
3. OpenSOHO → Discover → encontrar el dispositivo
4. Adopt con "Import existing config" → OpenSOHO detecta rol automáticamente
5. Usuario configura SSIDs, radios, DAWN en OpenSOHO
6. Push Config → OpenSOHO genera y aplica toda la configuración
```

---

## Estado actual (ya implementado)

### Colecciones DB existentes
- `devices` — dispositivos con `mac_address`, `last_ip_address`, `numradios`, `wifis`, `leds`, `enabled`, `config_status`, `health_status`
- `wifi` — SSIDs con todos los campos de roaming 802.11r/k: `ft_over_ds`, `ft_psk_generate_local`, `mobility_domain`, `ieee80211r_reassoc_deadline`, `nasid`, `rrm_neighbor_report`, `rrm_beacon_report`
- `radios` — radios con `band`, `channel`, `htmode`, `auto_frequency`
- `leds` — LEDs con triggers completos: `netdev` (dev+mode), `timer` (delayon+delayoff), `phy*`, etc.
- `dawn` — configuración DAWN completa (global, metric, times, behaviour)
- `vlan` — VLANs con `name`, `number`, `cidr`, `gateway`, `no_wan`, `no_lan`
- `bridges` — bridges por dispositivo (llenado por monitoring, solo lectura)
- `ethernet` — interfaces ethernet por dispositivo (llenado por monitoring, solo lectura)
- `settings` — configuración global (`country`)
- `port_tagging` — configuración de puertos por VLAN

### Generadores de config existentes en `opensoho.go`
- `generateWifiConfig` — genera `etc/config/wireless` (wifi-iface) con todos los campos de roaming
- `generateRadioConfig` — genera `etc/config/wireless` (wifi-device)
- `generateLedConfig` — genera `etc/config/system` (LEDs) con campos condicionales por trigger
- `generateDawnConfig` — genera `etc/config/dawn`
- `generateInterfacesConfig` — genera `etc/config/network` SOLO para VLANs DSA (requiere `apply=vlan` en device)
- `generateDhcpConfig` — genera `etc/config/dhcp` SOLO para VLANs con gateway

### Flujo SSH existente
- `ImportDeviceConfig` — lee `uci show wireless/system/dawn` y devuelve `ImportedConfig`
- `applyImportedConfig` — escribe radios, wifis, leds, dawn en DB
- `pushConfigAfterAdoption` — aplica config vía SSH tras adopción
- Handler de adopción soporta `import_config: bool`

### Frontend existente
- `PageSSHDevices.svelte` — lista de dispositivos, discovery, adopción con toggle import_config
- `PageSSHDeviceDetail.svelte` — detalle con WiFi, Radios, LEDs, DAWN
- `PageDawn.svelte` — configuración DAWN con 4 secciones acordeón
- `PageLeds.svelte` — gestión de LEDs con campos condicionales por trigger
- Rutas: `/ssh/devices`, `/ssh/device/:id`, `/ssh/dawn`, `/ssh/leds`

---

## Lo que falta implementar

### Secciones del script NO cubiertas actualmente

| Sección | Estado |
|---|---|
| `firewall disable` + neutralización | ❌ No implementado |
| `dnsmasq disable` / `odhcpd disable` | ❌ No implementado |
| `dhcp.@dnsmasq[0].disabled=1` | ❌ No implementado |
| `network` completo (br-lan, loopback, lan dhcp, vlan_invitados) | ⚠️ Parcial — solo DSA VLANs |
| Detección automática de rol (dumb_ap vs router) | ❌ No implementado |

---

## Plan de implementación

### Paso 1 — Migración DB: nueva colección `device_profile`

Crear colección `device_profile` con los siguientes campos:

| Campo | Tipo | Descripción |
|---|---|---|
| `name` | text (unique, required) | Ej: `dumb_ap`, `router` |
| `mode` | select: `dumb_ap` / `router` / `mesh_node` | Rol principal del dispositivo |
| `lan_proto` | select: `dhcp` / `static` / `none` | Protocolo de la interfaz LAN |
| `lan_cidr` | text | IP/máscara si `lan_proto=static` (ej: `192.168.1.1/24`) |
| `disable_firewall` | bool | Neutralizar firewall (dumb AP) |
| `disable_dnsmasq` | bool | Deshabilitar DHCP/DNS local |
| `disable_odhcpd` | bool | Deshabilitar DHCPv6 |
| `igmp_snooping` | bool | Activar IGMP snooping en br-lan |
| `bridge_vlan_filtering` | bool | Activar 802.1Q en br-lan |
| `stp` | bool | Activar STP en br-lan |
| `extra_networks` | json | Redes adicionales (ver formato abajo) |

**Formato `extra_networks` (JSON array):**
```json
[
  {
    "name": "vlan_invitados",
    "device": "br-invitados",
    "proto": "none",
    "bridge_ports": ["br-lan.10"]
  }
]
```

**Insertar dos registros por defecto en la migración:**
- `dumb_ap`: `mode=dumb_ap`, `lan_proto=dhcp`, `disable_firewall=true`, `disable_dnsmasq=true`, `disable_odhcpd=true`, `igmp_snooping=true`, `bridge_vlan_filtering=false`, `stp=false`
- `router`: `mode=router`, `lan_proto=static`, `disable_firewall=false`, `disable_dnsmasq=false`, `disable_odhcpd=false`

### Paso 2 — Migración DB: añadir `profile` a `devices`

Añadir campo `profile` (relación a `device_profile`, opcional, maxSelect=1) a la colección `devices`.

### Paso 3 — Nuevo generador `generateNetworkConfig` en `opensoho.go`

Reemplazar/extender `generateInterfacesConfig` para generar `etc/config/network` completo basado en el perfil del dispositivo.

**Lógica:**
1. Si el dispositivo no tiene perfil → usar `generateInterfacesConfig` actual (sin cambios)
2. Si tiene perfil → generar config completa:

```
# Loopback (siempre)
config interface 'loopback'
    option device 'lo'
    option proto 'static'
    option ipaddr '127.0.0.1'
    option netmask '255.0.0.0'

config globals 'globals'
    option ula_prefix 'auto'

# br-lan device (puertos desde colección ethernet del dispositivo)
config device 'br_lan_dev'
    option name 'br-lan'
    option type 'bridge'
    option bridge_vlan_filtering '0'  # desde perfil
    option stp '0'                    # desde perfil
    option igmp_snooping '1'          # desde perfil
    list ports 'eth0'                 # desde ethernet del dispositivo
    list ports 'eth1'                 # desde ethernet del dispositivo

# LAN interface
config interface 'lan'
    option device 'br-lan'
    option proto 'dhcp'               # desde perfil.lan_proto

# Extra networks desde perfil.extra_networks
config device 'br_invitados_dev'
    option name 'br-invitados'
    option type 'bridge'
    list ports 'br-lan.10'

config interface 'vlan_invitados'
    option device 'br-invitados'
    option proto 'none'
```

**Fuente de los puertos ethernet:** leer la colección `ethernet` filtrada por `device=record.Id`, usar los nombres de interfaz. Si no hay registros ethernet (dispositivo recién adoptado sin monitoring), usar los puertos del perfil o dejar vacío con un warning en el log.

### Paso 4 — Nuevo generador `generateFirewallConfig` en `opensoho.go`

Si `profile.disable_firewall=true`, generar `etc/config/firewall`:

```
config defaults
    option input 'ACCEPT'
    option output 'ACCEPT'
    option forward 'ACCEPT'
    option synflood_protect '0'
    option drop_invalid '0'
```

Si `disable_firewall=false` → no generar este archivo (OpenWRT mantiene su firewall por defecto).

### Paso 5 — Nuevo generador `generateDhcpProfileConfig` en `opensoho.go`

Renombrar/extender el `generateDhcpConfig` actual. Si el perfil tiene `disable_dnsmasq=true` o `disable_odhcpd=true`, generar `etc/config/dhcp`:

```
config dnsmasq
    option disabled '1'

config odhcpd 'odhcpd'
    option maindhcp '0'
```

Si ambos son false → usar el generador actual de DHCP para VLANs con gateway.

### Paso 6 — Integrar nuevos generadores en `GenerateDeviceConfigFiles`

Modificar `GenerateDeviceConfigFiles` en `opensoho.go`:

```go
// Cargar perfil del dispositivo
profile := loadDeviceProfile(app, record)

if profile != nil {
    // Generadores basados en perfil
    configfiles["etc/config/network"] = generateNetworkConfig(app, record, profile)
    if profile.GetBool("disable_firewall") {
        configfiles["etc/config/firewall"] = generateFirewallConfig(profile)
    }
    dhcpConfig := generateDhcpProfileConfig(app, record, profile)
    if dhcpConfig != "" {
        configfiles["etc/config/dhcp"] = dhcpConfig
    }
} else {
    // Comportamiento actual (sin perfil)
    if interfacesConfig := generateInterfacesConfig(app, record); interfacesConfig != "" {
        configfiles["etc/config/network"] = interfacesConfig
    }
    if dhcpConfig := generateDhcpConfig(app, record); dhcpConfig != "" {
        configfiles["etc/config/dhcp"] = dhcpConfig
    }
}
```

### Paso 7 — Actualizar `uci/builder.go`

Verificar que `ServiceReloadMap` incluye `firewall` y `dhcp`. Añadir soporte para comandos de disable de servicios como paso previo (antes del `uci commit`):

```go
var ServiceDisableMap = map[string]string{
    "firewall": "/etc/init.d/firewall disable 2>/dev/null; /etc/init.d/firewall stop 2>/dev/null",
    "dnsmasq":  "/etc/init.d/dnsmasq disable 2>/dev/null; /etc/init.d/dnsmasq stop 2>/dev/null",
    "odhcpd":   "/etc/init.d/odhcpd disable 2>/dev/null; /etc/init.d/odhcpd stop 2>/dev/null",
}
```

Modificar `ApplyConfigFiles` en `config/applier.go` para ejecutar los disable commands cuando el perfil lo requiera. Pasar el perfil como parámetro opcional o añadir un campo `pre_commands []string` al contexto de aplicación.

### Paso 8 — Detección automática de rol en `ImportDeviceConfig`

En `device/manager.go`, extender `ImportDeviceConfig` para detectar el rol del dispositivo leyendo `uci show network` y `uci show firewall`:

**Lógica de detección:**
- Leer `uci show firewall` → si `firewall.@defaults[0].input=ACCEPT` y no hay zonas → `dumb_ap`
- Leer `uci show network` → si `network.lan.proto=dhcp` → confirma `dumb_ap`
- Si tiene `network.wan` con proto distinto de none → `router`
- Default → `dumb_ap` (caso más común en SOHO)

Añadir al `ImportedConfig`:
```go
type ImportedConfig struct {
    // ... campos existentes ...
    DetectedRole    string            // "dumb_ap", "router", ""
    EthernetPorts   []string          // ["eth0", "eth1"] desde network.br_lan_dev.ports
    ExtraNetworks   []ImportedNetwork // redes adicionales detectadas
}

type ImportedNetwork struct {
    Name        string
    Device      string
    Proto       string
    BridgePorts []string
}
```

### Paso 9 — Actualizar `applyImportedConfig` en `ssh_integration.go`

Extender para:
1. Buscar o crear el perfil correspondiente al `DetectedRole`
2. Asignar el perfil al registro del dispositivo
3. Si hay `ExtraNetworks` detectadas, guardarlas en `profile.extra_networks`

### Paso 10 — Frontend: nueva página `PageProfiles.svelte`

Nueva página en `ui/src/components/ssh/PageProfiles.svelte`:

**Ruta:** `/ssh/profiles`

**Contenido:**
- Lista de perfiles con nombre, modo y resumen de configuración
- Botón **New Profile** → panel lateral con formulario
- Formulario con campos condicionales:
  - Siempre: `name`, `mode`, `lan_proto`
  - Si `mode=dumb_ap`: checkboxes `disable_firewall`, `disable_dnsmasq`, `disable_odhcpd`, `igmp_snooping`, `bridge_vlan_filtering`, `stp`
  - Si `lan_proto=static`: campo `lan_cidr`
  - Sección **Extra Networks**: tabla editable con columnas `name`, `device`, `proto`, `bridge_ports` (lista separada por comas)
- Botón eliminar con confirmación (solo si ningún dispositivo usa el perfil)

### Paso 11 — Frontend: selector de perfil en `PageSSHDeviceDetail.svelte`

En la sección **Device Info**, añadir:
- Select de perfil con los perfiles disponibles (cargados de la colección `device_profile`)
- Al seleccionar un perfil, mostrar un resumen expandible: "Este perfil configurará: firewall neutralizado, LAN por DHCP, redes adicionales: vlan_invitados"
- Botón **Apply Profile + Push Config** que asigna el perfil y hace push inmediato

### Paso 12 — Frontend: añadir link a Profiles en `PageSSHDevices.svelte`

Añadir botón **Profiles** en el header junto a LEDs y DAWN:
```svelte
<a href="/ssh/profiles" class="btn btn-outline" use:link>
    <i class="ri-layout-grid-line" />
    <span class="txt">Profiles</span>
</a>
```

### Paso 13 — Frontend: registrar ruta en `routes.js`

```js
import PageProfiles from "@/components/ssh/PageProfiles.svelte";

"/ssh/profiles": wrap({
    component: PageProfiles,
    conditions: [(_) => ApiClient.authStore.isValid],
    userData: { showAppSidebar: true },
}),
```

---

## Dependencias entre pasos

```
1 (migración device_profile)
2 (migración devices.profile)  → depende de 1
3 (generateNetworkConfig)      → depende de 1, 2
4 (generateFirewallConfig)     → depende de 1
5 (generateDhcpProfileConfig)  → depende de 1
6 (integrar generadores)       → depende de 3, 4, 5
7 (uci/builder.go)             → independiente
8 (ImportDeviceConfig)         → independiente
9 (applyImportedConfig)        → depende de 1, 8
10 (PageProfiles.svelte)       → depende de 1
11 (PageSSHDeviceDetail)       → depende de 1, 2
12 (PageSSHDevices header)     → depende de 10
13 (routes.js)                 → depende de 10
```

---

## Consideraciones importantes

### Puertos ethernet en br-lan
Los puertos físicos (`eth0`, `eth1`) se conocen después del primer ciclo de monitoring (colección `ethernet`). Para el primer push tras adopción sin monitoring previo, el generador debe:
1. Intentar leer de la colección `ethernet` del dispositivo
2. Si está vacía, leer los puertos del `ImportedConfig.EthernetPorts` (detectados durante import)
3. Si tampoco hay datos, omitir la lista `ports` del bridge (OpenWRT usará todos los puertos disponibles por defecto)

### Compatibilidad con dispositivos sin perfil
Los dispositivos existentes sin perfil asignado deben seguir funcionando exactamente igual que antes. El perfil es completamente opcional — si `profile` es null, se usa el comportamiento actual.

### Idempotencia del push
El `ApplyConfigFiles` ya usa diff UCI, por lo que aplicar la misma config dos veces no tiene efecto. Los comandos de disable de servicios son idempotentes por naturaleza.

### VLAN `vlan_invitados` y la wifi `invitados`
Para que la wifi `invitados` use `network=vlan_invitados`, la VLAN debe existir en OpenSOHO con ese nombre exacto. El generador `getVlan` ya busca por nombre en la colección `vlan`. Si se crea la VLAN `vlan_invitados` en OpenSOHO y se asigna a la wifi `invitados`, el generador la usará automáticamente.

---

## Archivos a modificar/crear

| Archivo | Acción |
|---|---|
| `pb_migrations/TIMESTAMP_created_device_profile.js` | Crear |
| `pb_migrations/TIMESTAMP_updated_devices_profile.js` | Crear |
| `opensoho.go` | Modificar: añadir `generateNetworkConfig`, `generateFirewallConfig`, `generateDhcpProfileConfig`, `loadDeviceProfile`, actualizar `GenerateDeviceConfigFiles` |
| `uci/builder.go` | Modificar: añadir `ServiceDisableMap` |
| `config/applier.go` | Modificar: soportar pre_commands de disable |
| `device/manager.go` | Modificar: extender `ImportDeviceConfig` con detección de rol y puertos |
| `ssh_integration.go` | Modificar: extender `applyImportedConfig` con asignación de perfil |
| `ui/src/components/ssh/PageProfiles.svelte` | Crear |
| `ui/src/components/ssh/PageSSHDeviceDetail.svelte` | Modificar: añadir selector de perfil |
| `ui/src/components/ssh/PageSSHDevices.svelte` | Modificar: añadir link a Profiles |
| `ui/src/routes.js` | Modificar: añadir ruta `/ssh/profiles` |

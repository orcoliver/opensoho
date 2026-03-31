/// <reference path="../pb_data/types.d.ts" />
migrate((app) => {
  const collection = new Collection({
    "createRule": "@request.auth.collectionName = \"_superusers\" && @request.auth.id != \"\"",
    "deleteRule": "@request.auth.collectionName = \"_superusers\" && @request.auth.id != \"\"",
    "listRule": "@request.auth.collectionName = \"_superusers\" && @request.auth.id != \"\"",
    "updateRule": "@request.auth.collectionName = \"_superusers\" && @request.auth.id != \"\"",
    "viewRule": "@request.auth.collectionName = \"_superusers\" && @request.auth.id != \"\"",
    "fields": [
      {
        "autogeneratePattern": "[a-z0-9]{15}",
        "hidden": false,
        "id": "text3208210256",
        "max": 15,
        "min": 15,
        "name": "id",
        "pattern": "^[a-z0-9]+$",
        "presentable": false,
        "primaryKey": true,
        "required": true,
        "system": true,
        "type": "text"
      },
      {
        "autogeneratePattern": "",
        "hidden": false,
        "id": "text_dp_name",
        "max": 64,
        "min": 1,
        "name": "name",
        "pattern": "",
        "presentable": true,
        "primaryKey": false,
        "required": true,
        "system": false,
        "type": "text"
      },
      {
        "hidden": false,
        "id": "select_dp_mode",
        "maxSelect": 1,
        "name": "mode",
        "presentable": false,
        "required": true,
        "system": false,
        "type": "select",
        "values": ["dumb_ap", "router", "mesh_node"]
      },
      {
        "hidden": false,
        "id": "select_dp_lan_proto",
        "maxSelect": 1,
        "name": "lan_proto",
        "presentable": false,
        "required": true,
        "system": false,
        "type": "select",
        "values": ["dhcp", "static", "none"]
      },
      {
        "autogeneratePattern": "",
        "hidden": false,
        "id": "text_dp_lan_cidr",
        "max": 0,
        "min": 0,
        "name": "lan_cidr",
        "pattern": "",
        "presentable": false,
        "primaryKey": false,
        "required": false,
        "system": false,
        "type": "text"
      },
      {
        "hidden": false,
        "id": "bool_dp_disable_firewall",
        "name": "disable_firewall",
        "presentable": false,
        "required": false,
        "system": false,
        "type": "bool"
      },
      {
        "hidden": false,
        "id": "bool_dp_disable_dnsmasq",
        "name": "disable_dnsmasq",
        "presentable": false,
        "required": false,
        "system": false,
        "type": "bool"
      },
      {
        "hidden": false,
        "id": "bool_dp_disable_odhcpd",
        "name": "disable_odhcpd",
        "presentable": false,
        "required": false,
        "system": false,
        "type": "bool"
      },
      {
        "hidden": false,
        "id": "bool_dp_igmp_snooping",
        "name": "igmp_snooping",
        "presentable": false,
        "required": false,
        "system": false,
        "type": "bool"
      },
      {
        "hidden": false,
        "id": "bool_dp_bridge_vlan_filtering",
        "name": "bridge_vlan_filtering",
        "presentable": false,
        "required": false,
        "system": false,
        "type": "bool"
      },
      {
        "hidden": false,
        "id": "bool_dp_stp",
        "name": "stp",
        "presentable": false,
        "required": false,
        "system": false,
        "type": "bool"
      },
      {
        "hidden": false,
        "id": "json_dp_extra_networks",
        "maxSize": 0,
        "name": "extra_networks",
        "presentable": false,
        "required": false,
        "system": false,
        "type": "json"
      },
      {
        "hidden": false,
        "id": "autodate2990389176",
        "name": "created",
        "onCreate": true,
        "onUpdate": false,
        "presentable": false,
        "system": false,
        "type": "autodate"
      },
      {
        "hidden": false,
        "id": "autodate3332085495",
        "name": "updated",
        "onCreate": true,
        "onUpdate": true,
        "presentable": false,
        "system": false,
        "type": "autodate"
      }
    ],
    "id": "pbc_device_profile",
    "indexes": [
      "CREATE UNIQUE INDEX idx_dp_name ON device_profile (name)"
    ],
    "name": "device_profile",
    "system": false,
    "type": "base"
  })

  app.save(collection)

  // Insert default profiles
  const dumbAp = new Record(collection)
  dumbAp.set("name", "dumb_ap")
  dumbAp.set("mode", "dumb_ap")
  dumbAp.set("lan_proto", "dhcp")
  dumbAp.set("disable_firewall", true)
  dumbAp.set("disable_dnsmasq", true)
  dumbAp.set("disable_odhcpd", true)
  dumbAp.set("igmp_snooping", true)
  dumbAp.set("bridge_vlan_filtering", false)
  dumbAp.set("stp", false)
  app.save(dumbAp)

  const router = new Record(collection)
  router.set("name", "router")
  router.set("mode", "router")
  router.set("lan_proto", "static")
  router.set("disable_firewall", false)
  router.set("disable_dnsmasq", false)
  router.set("disable_odhcpd", false)
  router.set("igmp_snooping", false)
  router.set("bridge_vlan_filtering", false)
  router.set("stp", false)
  app.save(router)

}, (app) => {
  const collection = app.findCollectionByNameOrId("pbc_device_profile")
  return app.delete(collection)
})

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
      // --- global ---
      { "hidden": false, "id": "bool_dawn_kicking", "name": "kicking", "presentable": false, "required": false, "system": false, "type": "bool" },
      { "hidden": false, "id": "bool_dawn_set_hostapd_nr", "name": "set_hostapd_nr", "presentable": false, "required": false, "system": false, "type": "bool" },
      {
        "hidden": false, "id": "select_dawn_rrm_mode", "maxSelect": 1, "name": "rrm_mode",
        "presentable": false, "required": false, "system": false, "type": "select",
        "values": ["pat", "all", "off"]
      },
      // --- metric ---
      { "hidden": false, "id": "number_dawn_initial_score", "name": "initial_score", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_ht_support", "name": "ht_support", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_vht_support", "name": "vht_support", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_he_support", "name": "he_support", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_rssi", "name": "rssi", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_rssi_val", "name": "rssi_val", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_low_rssi", "name": "low_rssi", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_low_rssi_val", "name": "low_rssi_val", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_freq_5", "name": "freq_5", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_chan_util", "name": "chan_util", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_rssi_weight", "name": "rssi_weight", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_rssi_center", "name": "rssi_center", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      // --- times ---
      { "hidden": false, "id": "number_dawn_update_client", "name": "update_client", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_remove_client", "name": "remove_client", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_remove_probe", "name": "remove_probe", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_update_hostapd", "name": "update_hostapd", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_update_tcp_con", "name": "update_tcp_con", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_update_chan_util", "name": "update_chan_util", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_update_beacon_reports", "name": "update_beacon_reports", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      // --- behaviour ---
      { "hidden": false, "id": "number_dawn_kicking_threshold", "name": "kicking_threshold", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_min_probe_count", "name": "min_probe_count", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_bandwidth_threshold", "name": "bandwidth_threshold", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "bool_dawn_use_station_count", "name": "use_station_count", "presentable": false, "required": false, "system": false, "type": "bool" },
      { "hidden": false, "id": "number_dawn_max_station_diff", "name": "max_station_diff", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_min_number_to_kick", "name": "min_number_to_kick", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_chan_util_avg_period", "name": "chan_util_avg_period", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      { "hidden": false, "id": "number_dawn_min_kick_count", "name": "min_kick_count", "onlyInt": true, "presentable": false, "required": false, "system": false, "type": "number" },
      {
        "hidden": false, "id": "autodate2990389176", "name": "created",
        "onCreate": true, "onUpdate": false, "presentable": false, "system": false, "type": "autodate"
      },
      {
        "hidden": false, "id": "autodate3332085495", "name": "updated",
        "onCreate": true, "onUpdate": true, "presentable": false, "system": false, "type": "autodate"
      }
    ],
    "id": "pbc_dawn_001",
    "indexes": [],
    "name": "dawn",
    "system": false,
    "type": "base"
  })

  return app.save(collection)
}, (app) => {
  const collection = app.findCollectionByNameOrId("pbc_dawn_001")
  return app.delete(collection)
})

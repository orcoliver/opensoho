/// <reference path="../pb_data/types.d.ts" />
migrate((app) => {
  const collection = app.findCollectionByNameOrId("pbc_4047009785")

  // Update the existing trigger select to include all OpenWRT triggers
  collection.fields.addAt(4, new Field({
    "hidden": false,
    "id": "select4173031077",
    "maxSelect": 1,
    "name": "trigger",
    "presentable": false,
    "required": false,
    "system": false,
    "type": "select",
    "values": [
      "none",
      "default-on",
      "heartbeat",
      "timer",
      "netdev",
      "phy0rx",
      "phy0tx",
      "phy0assoc",
      "phy0radio",
      "phy0tpt",
      "phy1rx",
      "phy1tx",
      "phy1assoc",
      "phy1radio",
      "phy1tpt"
    ]
  }))

  // netdev: network interface to watch
  collection.fields.addAt(5, new Field({
    "autogeneratePattern": "",
    "hidden": false,
    "id": "text_led_dev",
    "max": 16,
    "min": 0,
    "name": "dev",
    "pattern": "",
    "presentable": false,
    "primaryKey": false,
    "required": false,
    "system": false,
    "type": "text"
  }))

  // netdev: link/tx/rx modes (space-separated, e.g. "link tx rx")
  collection.fields.addAt(6, new Field({
    "hidden": false,
    "id": "select_led_mode",
    "maxSelect": 3,
    "name": "mode",
    "presentable": false,
    "required": false,
    "system": false,
    "type": "select",
    "values": [
      "link",
      "tx",
      "rx"
    ]
  }))

  // timer: milliseconds on
  collection.fields.addAt(7, new Field({
    "hidden": false,
    "id": "number_led_delayon",
    "max": null,
    "min": 0,
    "name": "delayon",
    "onlyInt": true,
    "presentable": false,
    "required": false,
    "system": false,
    "type": "number"
  }))

  // timer: milliseconds off
  collection.fields.addAt(8, new Field({
    "hidden": false,
    "id": "number_led_delayoff",
    "max": null,
    "min": 0,
    "name": "delayoff",
    "onlyInt": true,
    "presentable": false,
    "required": false,
    "system": false,
    "type": "number"
  }))

  return app.save(collection)
}, (app) => {
  const collection = app.findCollectionByNameOrId("pbc_4047009785")

  // Restore original trigger select
  collection.fields.addAt(4, new Field({
    "hidden": false,
    "id": "select4173031077",
    "maxSelect": 1,
    "name": "trigger",
    "presentable": false,
    "required": false,
    "system": false,
    "type": "select",
    "values": [
      "default-on",
      "none"
    ]
  }))

  collection.fields.removeById("text_led_dev")
  collection.fields.removeById("select_led_mode")
  collection.fields.removeById("number_led_delayon")
  collection.fields.removeById("number_led_delayoff")

  return app.save(collection)
})

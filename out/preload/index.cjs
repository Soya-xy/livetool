"use strict";
const electron = require("electron");
const api = {
  rules: {
    list: () => electron.ipcRenderer.invoke("rules:list"),
    save: (rule) => electron.ipcRenderer.invoke("rules:save", rule),
    remove: (id) => electron.ipcRenderer.invoke("rules:remove", id),
    clear: () => electron.ipcRenderer.invoke("rules:clear"),
    clone: (id) => electron.ipcRenderer.invoke("rules:clone", id)
  },
  danmaku: {
    query: (filter) => electron.ipcRenderer.invoke("danmaku:query", filter),
    count: (filter) => electron.ipcRenderer.invoke("danmaku:count", filter),
    export: (filter, format) => electron.ipcRenderer.invoke("danmaku:export", { filter, format }),
    clear: (filter) => electron.ipcRenderer.invoke("danmaku:clear", filter),
    onAppend: (callback) => subscribe("danmaku:append", callback)
  },
  conn: {
    connect: (platform, roomId) => electron.ipcRenderer.invoke("conn:connect", platform, roomId),
    disconnect: () => electron.ipcRenderer.invoke("conn:disconnect"),
    status: () => electron.ipcRenderer.invoke("conn:status"),
    simulate: (event) => electron.ipcRenderer.invoke("conn:simulate", event),
    onStatus: (callback) => subscribe("conn:status", callback)
  },
  overlay: {
    open: (type) => electron.ipcRenderer.invoke("overlay:open", type),
    close: (type) => electron.ipcRenderer.invoke("overlay:close", type),
    status: () => electron.ipcRenderer.invoke("overlay:status"),
    setMode: (type, mode) => electron.ipcRenderer.invoke("overlay:setMode", type, mode),
    toggleOpacity: (type) => electron.ipcRenderer.invoke("overlay:toggleOpacity", type),
    updateSettings: (type, settings) => electron.ipcRenderer.invoke("overlay:updateSettings", type, settings),
    playVideo: (payload) => electron.ipcRenderer.invoke("overlay:playVideo", payload),
    drop: (payload) => electron.ipcRenderer.invoke("overlay:drop", payload),
    slot: (payload) => electron.ipcRenderer.invoke("overlay:slot", payload),
    widget: (payload) => electron.ipcRenderer.invoke("overlay:widget", payload),
    removeWidget: (featureId) => electron.ipcRenderer.invoke("overlay:removeWidget", featureId),
    onMessage: (callback) => subscribe("overlay:message", callback),
    onStatus: (callback) => subscribe("overlay:status", callback)
  },
  audio: {
    play: (payload) => electron.ipcRenderer.invoke("audio:play", payload),
    stop: () => electron.ipcRenderer.invoke("audio:stop"),
    onMessage: (callback) => subscribe("audio:message", callback)
  },
  input: {
    run: (action) => electron.ipcRenderer.invoke("input:run", action),
    findImage: (image, threshold) => electron.ipcRenderer.invoke("input:findImage", image, threshold)
  },
  serial: {
    ports: () => electron.ipcRenderer.invoke("serial:ports"),
    pulse: (action) => electron.ipcRenderer.invoke("serial:pulse", action),
    stopAll: () => electron.ipcRenderer.invoke("serial:stopAll")
  },
  obs: {
    connect: (url, password) => electron.ipcRenderer.invoke("obs:connect", url, password),
    command: (command, args) => electron.ipcRenderer.invoke("obs:command", command, args),
    startVirtualCamera: () => electron.ipcRenderer.invoke("obs:startVirtualCamera"),
    stopVirtualCamera: () => electron.ipcRenderer.invoke("obs:stopVirtualCamera"),
    status: () => electron.ipcRenderer.invoke("obs:status")
  },
  features: {
    test: (featureId) => electron.ipcRenderer.invoke("features:test", featureId),
    show: (featureId) => electron.ipcRenderer.invoke("features:show", featureId),
    increment: (featureId, key, amount) => electron.ipcRenderer.invoke("features:increment", featureId, key, amount)
  },
  auth: {
    status: () => electron.ipcRenderer.invoke("auth:status"),
    login: (payload) => electron.ipcRenderer.invoke("auth:login", payload),
    logout: () => electron.ipcRenderer.invoke("auth:logout"),
    setSafeCode: (code) => electron.ipcRenderer.invoke("auth:setSafeCode", code),
    unbind: (safeCode) => electron.ipcRenderer.invoke("auth:unbind", safeCode),
    adminStatus: () => electron.ipcRenderer.invoke("auth:adminStatus"),
    adminLogin: (payload) => electron.ipcRenderer.invoke("auth:adminLogin", payload),
    adminLogout: () => electron.ipcRenderer.invoke("auth:adminLogout"),
    listCards: (query) => electron.ipcRenderer.invoke("auth:listCards", query),
    createCards: (input) => electron.ipcRenderer.invoke("auth:createCards", input),
    setCardStatus: (id, status) => electron.ipcRenderer.invoke("auth:setCardStatus", id, status),
    resetCardBindings: (id) => electron.ipcRenderer.invoke("auth:resetCardBindings", id),
    cardDetail: (id) => electron.ipcRenderer.invoke("auth:cardDetail", id),
    extendCard: (id, days) => electron.ipcRenderer.invoke("auth:extendCard", id, days),
    auditLog: (limit) => electron.ipcRenderer.invoke("auth:auditLog", limit)
  },
  config: {
    export: () => electron.ipcRenderer.invoke("config:export"),
    import: () => electron.ipcRenderer.invoke("config:import")
  },
  diagnostics: {
    logs: (limit) => electron.ipcRenderer.invoke("diagnostics:logs", limit),
    assets: () => electron.ipcRenderer.invoke("diagnostics:assets"),
    settings: () => electron.ipcRenderer.invoke("diagnostics:settings"),
    saveSettings: (settings) => electron.ipcRenderer.invoke("diagnostics:saveSettings", settings),
    onLog: (callback) => subscribe("log:append", callback)
  },
  window: {
    minimize: () => electron.ipcRenderer.invoke("window:minimize"),
    maximize: () => electron.ipcRenderer.invoke("window:maximize"),
    close: () => electron.ipcRenderer.invoke("window:close")
  }
};
electron.contextBridge.exposeInMainWorld("api", api);
function subscribe(channel, callback) {
  const listener = (_event, payload) => callback(payload);
  electron.ipcRenderer.on(channel, listener);
  return () => electron.ipcRenderer.removeListener(channel, listener);
}

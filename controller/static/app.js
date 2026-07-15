function md5Hex(e) {
    function t(e, t) {
        return e << t | e >>> 32 - t
    }

    function n(e, t) {
        var n, o, a, i, s;
        return a = 2147483648 & e, i = 2147483648 & t, s = (1073741823 & e) + (1073741823 & t), (n = 1073741824 & e) & (o = 1073741824 & t) ? 2147483648 ^ s ^ a ^ i : n | o ? 1073741824 & s ? 3221225472 ^ s ^ a ^ i : 1073741824 ^ s ^ a ^ i : s ^ a ^ i
    }

    function o(e, o, a, i, s, c, r) {
        return e = n(e, n(n(function(e, t, n) {
            return e & t | ~e & n
        }(o, a, i), s), r)), n(t(e, c), o)
    }

    function a(e, o, a, i, s, c, r) {
        return e = n(e, n(n(function(e, t, n) {
            return e & n | t & ~n
        }(o, a, i), s), r)), n(t(e, c), o)
    }

    function i(e, o, a, i, s, c, r) {
        return e = n(e, n(n(function(e, t, n) {
            return e ^ t ^ n
        }(o, a, i), s), r)), n(t(e, c), o)
    }

    function s(e, o, a, i, s, c, r) {
        return e = n(e, n(n(function(e, t, n) {
            return t ^ (e | ~n)
        }(o, a, i), s), r)), n(t(e, c), o)
    }

    function c(e) {
        var t, n = "",
            o = "";
        for (t = 0; t <= 3; t++) n += (o = "0" + (e >>> 8 * t & 255).toString(16)).substr(o.length - 2, 2);
        return n
    }
    var r, d, l, p, u, m, f, y, h, g = function(e) {
        for (var t, n = e.length, o = n + 8, a = 16 * ((o - o % 64) / 64 + 1), i = new Array(a - 1), s = 0, c = 0; c < n;) s = c % 4 * 8, i[t = (c - c % 4) / 4] = i[t] | e.charCodeAt(c) << s, c++;
        return s = c % 4 * 8, i[t = (c - c % 4) / 4] = i[t] | 128 << s, i[a - 2] = n << 3, i[a - 1] = n >>> 29, i
    }(e);
    for (m = 1732584193, f = 4023233417, y = 2562383102, h = 271733878, r = 0; r < g.length; r += 16) d = m, l = f, p = y, u = h, m = o(m, f, y, h, g[r + 0], 7, 3614090360), h = o(h, m, f, y, g[r + 1], 12, 3905402710), y = o(y, h, m, f, g[r + 2], 17, 606105819), f = o(f, y, h, m, g[r + 3], 22, 3250441966), m = o(m, f, y, h, g[r + 4], 7, 4118548399), h = o(h, m, f, y, g[r + 5], 12, 1200080426), y = o(y, h, m, f, g[r + 6], 17, 2821735955), f = o(f, y, h, m, g[r + 7], 22, 4249261313), m = o(m, f, y, h, g[r + 8], 7, 1770035416), h = o(h, m, f, y, g[r + 9], 12, 2336552879), y = o(y, h, m, f, g[r + 10], 17, 4294925233), f = o(f, y, h, m, g[r + 11], 22, 2304563134), m = o(m, f, y, h, g[r + 12], 7, 1804603682), h = o(h, m, f, y, g[r + 13], 12, 4254626195), y = o(y, h, m, f, g[r + 14], 17, 2792965006), m = a(m, f = o(f, y, h, m, g[r + 15], 22, 1236535329), y, h, g[r + 1], 5, 4129170786), h = a(h, m, f, y, g[r + 6], 9, 3225465664), y = a(y, h, m, f, g[r + 11], 14, 643717713), f = a(f, y, h, m, g[r + 0], 20, 3921069994), m = a(m, f, y, h, g[r + 5], 5, 3593408605), h = a(h, m, f, y, g[r + 10], 9, 38016083), y = a(y, h, m, f, g[r + 15], 14, 3634488961), f = a(f, y, h, m, g[r + 4], 20, 3889429448), m = a(m, f, y, h, g[r + 9], 5, 568446438), h = a(h, m, f, y, g[r + 14], 9, 3275163606), y = a(y, h, m, f, g[r + 3], 14, 4107603335), f = a(f, y, h, m, g[r + 8], 20, 1163531501), m = a(m, f, y, h, g[r + 13], 5, 2850285829), h = a(h, m, f, y, g[r + 2], 9, 4243563512), y = a(y, h, m, f, g[r + 7], 14, 1735328473), m = i(m, f = a(f, y, h, m, g[r + 12], 20, 2368359562), y, h, g[r + 5], 4, 4294588738), h = i(h, m, f, y, g[r + 8], 11, 2272392833), y = i(y, h, m, f, g[r + 11], 16, 1839030562), f = i(f, y, h, m, g[r + 14], 23, 4259657740), m = i(m, f, y, h, g[r + 1], 4, 2763975236), h = i(h, m, f, y, g[r + 4], 11, 1272893353), y = i(y, h, m, f, g[r + 7], 16, 4139469664), f = i(f, y, h, m, g[r + 10], 23, 3200236656), m = i(m, f, y, h, g[r + 13], 4, 681279174), h = i(h, m, f, y, g[r + 0], 11, 3936430074), y = i(y, h, m, f, g[r + 3], 16, 3572445317), f = i(f, y, h, m, g[r + 6], 23, 76029189), m = i(m, f, y, h, g[r + 9], 4, 3654602809), h = i(h, m, f, y, g[r + 12], 11, 3873151461), y = i(y, h, m, f, g[r + 15], 16, 530742520), m = s(m, f = i(f, y, h, m, g[r + 2], 23, 3299628645), y, h, g[r + 0], 6, 4096336452), h = s(h, m, f, y, g[r + 7], 10, 1126891415), y = s(y, h, m, f, g[r + 14], 15, 2878612391), f = s(f, y, h, m, g[r + 5], 21, 4237533241), m = s(m, f, y, h, g[r + 12], 6, 1700485571), h = s(h, m, f, y, g[r + 3], 10, 2399980690), y = s(y, h, m, f, g[r + 10], 15, 4293915773), f = s(f, y, h, m, g[r + 1], 21, 2240044497), m = s(m, f, y, h, g[r + 8], 6, 1873313359), h = s(h, m, f, y, g[r + 15], 10, 4264355552), y = s(y, h, m, f, g[r + 6], 15, 2734768916), f = s(f, y, h, m, g[r + 13], 21, 1309151649), m = s(m, f, y, h, g[r + 4], 6, 4149444226), h = s(h, m, f, y, g[r + 11], 10, 3174756917), y = s(y, h, m, f, g[r + 2], 15, 718787259), f = s(f, y, h, m, g[r + 9], 21, 3951481745), m = n(m, d), f = n(f, l), y = n(y, p), h = n(h, u);
    return (c(m) + c(f) + c(y) + c(h)).toLowerCase()
}
let devices = [],
    networks = [],
    links = [],
    accounts = [],
    relays = [],
    refreshTimer = null;

function ago(e) {
    const t = new Date(e).getTime();
    if (!t || new Date(e).getFullYear() < 2e3) return "从未";
    const n = Math.floor((Date.now() - t) / 1e3);
    return n < 60 ? n + " 秒前" : n < 3600 ? Math.floor(n / 60) + " 分钟前" : n < 86400 ? Math.floor(n / 3600) + " 小时前" : Math.floor(n / 86400) + " 天前"
}

function esc(e) {
    return String(e ?? "").replace(/[&<>"]/g, e => ({
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;"
    } [e]))
}
async function api(e, t) {
    const n = await fetch(e, t);
    if (401 === n.status) return showLogin(), !1;
    if (!n.ok && 204 !== n.status) {
        const e = (await n.text()).trim();
        return alert("操作失败 (" + n.status + "): " + e), !1
    }
    return !0
}
async function del(e, t) {
    confirm("确认移除设备「" + t + "」?\n移除后该设备需重新注册,虚拟 IP 会被回收。") && (await api("/api/devices/" + e, {
        method: "DELETE"
    }), refresh())
}
async function kickDevice(e, t) {
    confirm("确认强制下线设备「" + t + "」?\n(仅标记为离线,下次心跳会重新上线)") && (await api("/api/devices/" + e + "/kick", {
        method: "POST"
    }), refresh())
}
async function disableDevice(e, t) {
    confirm("确认禁用设备「" + t + "」?\n禁用后该设备及其相同 MAC 的机器将无法注册/心跳,直到手动启用。") && (await api("/api/devices/" + e + "/disable", {
        method: "POST"
    }), refresh())
}
async function enableDevice(e, t) {
    confirm("确认启用设备「" + t + "」?") && (await api("/api/devices/" + e + "/enable", {
        method: "POST"
    }), refresh())
}
async function setNetwork(e, t) {
    await api("/api/devices/" + e, {
        method: "PUT",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify({
            network: t
        })
    }), refresh()
}
async function setIP(e, t) {
    openModal("手动指定虚拟 IP", {
        value: t,
        placeholder: "需在所属网络网段内",
        hint: "留空或相同则取消",
        onOK: async n => {
            closeModal(), n && n !== t && (await api("/api/devices/" + e, {
                method: "PUT",
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify({
                    virtual_ip: n
                })
            }), refresh())
        }
    })
}
async function addNetwork() {
    const e = document.getElementById("net-name").value.trim(),
        t = document.getElementById("net-cidr").value.trim();
    e && t ? (await api("/api/networks", {
        method: "POST",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify({
            name: e,
            cidr: t
        })
    }) && (document.getElementById("net-name").value = "", document.getElementById("net-cidr").value = ""), refresh()) : alert("请填写网络名称和 CIDR")
}
async function delNetwork(e, t) {
    confirm("确认删除网络「" + t + "」?") && (await api("/api/networks/" + e, {
        method: "DELETE"
    }), refresh())
}
async function addLink(e) {
    const t = "network" === e ? "link-net" : "link-dev",
        n = document.getElementById(t + "-a").value,
        o = document.getElementById(t + "-b").value;
    n && o ? (await api("/api/links", {
        method: "POST",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify({
            type: e,
            a: n,
            b: o
        })
    }), refresh()) : alert("请选择两端")
}
async function connectAllNetworks() {
    const e = document.getElementById("link-hub").value;
    if (!e) return void alert("请选择网络");
    const t = networks.filter(t => t.id !== e);
    if (!t.length) return void alert("没有其他网络可连通");
    const n = (e, t) => links.some(n => "network" === n.type && (n.a === e && n.b === t || n.a === t && n.b === e)),
        o = [];
    for (const a of t)
        if (!n(e, a.id)) try {
            const t = await fetch("/api/links", {
                method: "POST",
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify({
                    type: "network",
                    a: e,
                    b: a.id
                })
            });
            t.ok || 204 === t.status || o.push(a.name + "(" + (await t.text()).trim() + ")")
        } catch (e) {
            o.push(a.name)
        }
    refresh(), o.length && alert("以下网络连通失败:\n" + o.join("\n"))
}
async function delLink(e) {
    await api("/api/links/" + e, {
        method: "DELETE"
    }), refresh()
}

function fillLinkSelects() {
    fillSelects(["link-net-a", "link-net-b", "link-hub"], networks.map(e => [e.id, e.name + " (" + e.cidr + ")"])), fillSelects(["link-dev-a", "link-dev-b"], devices.map(e => [e.id, (e.name || e.virtual_ip) + " (" + e.virtual_ip + ")"]))
}

function fillSelects(e, t) {
    for (const n of e) {
        const e = document.getElementById(n),
            o = e.value;
        e.innerHTML = t.map(([e, t]) => `<option value="${esc(e)}">${esc(t)}</option>`).join(""), [...e.options].some(e => e.value === o) && (e.value = o)
    }
}

function netName(e) {
    const t = networks.find(t => t.id === e);
    return t ? t.name : e
}

function lanHTML(e) {
    return (e.lan_subnets || []).map(e => `<div class="mono">${esc(e)}</div>`).join("") + (e.lan_conflicts || []).map(e => `<div class="mono lan-bad">${esc(e)}(冲突)</div>`).join("") || "-"
}

function latHTML(e, t) {
    if (!e.latencies || !e.latency_updated_at) return "-";
    if (Date.now() - new Date(e.latency_updated_at).getTime() > 12e4) return "-";
    const n = Object.entries(e.latencies).sort((e, t) => e[0] < t[0] ? -1 : 1);
    return n.length ? n.map(([e, n]) => `<div class="lat">→ ${esc(t[e]||e)}: ${Math.round(n)}ms</div>`).join("") : "-"
}

function filteredDevices() {
    const e = document.getElementById("dev-search").value.trim().toLowerCase(),
        t = document.getElementById("dev-filter-net").value,
        d = document.getElementById("dev-filter-disabled").checked;
    return devices.filter(n => {
        if (d && !n.disabled) return !1;
        if (t && n.network !== t) return !1;
        if (e) {
            if (!((n.name || "") + " " + (n.virtual_ip || "")).toLowerCase().includes(e)) return !1
        }
        return !0
    })
}

function renderDevices() {
    document.getElementById("total").textContent = devices.length, document.getElementById("online").textContent = devices.filter(e => e.online).length, document.getElementById("offline").textContent = devices.filter(e => !e.online).length, document.getElementById("disabled-count").textContent = devices.filter(e => e.disabled).length;
    const e = {};
    devices.forEach(t => e[t.virtual_ip] = t.name || t.virtual_ip);
    const t = document.getElementById("tbody"),
        n = filteredDevices();
    n.length ? t.innerHTML = n.map(t => {
        const n = t.disabled,
            o = n ? '<span class="dot" style="background:#7f1d1d"></span>已禁用' : t.online ? '<span class="dot on"></span>在线' : '<span class="dot off"></span>离线',
            a = t.macs && t.macs.length ? t.macs.join("\\n") : "无 MAC 记录",
            i = t.machine_code || "-";
        return `\n    <tr class="${n?"disabled-row":""}">\n      <td>${o}</td>\n      <td>${esc(t.name)||"-"}</td>\n      <td>${esc(t.username)||"-"}</td>\n      <td class="ip" title="点击修改" onclick="setIP('${esc(t.id)}','${esc(t.virtual_ip)}')">${esc(t.virtual_ip)}</td>\n      <td>${esc(t.platform)||"-"}</td>\n      <td><select onchange="setNetwork('${esc(t.id)}', this.value)">${(e=>networks.map(t=>`<option value="${esc(t.name)}" ${t.id===e.network?"selected":""}>${esc(t.name)}</option>`).join(""))(t)}</select></td>\n      <td>${lanHTML(t)}</td>\n      <td title="${esc(a)}" style="cursor:help;font-family:Consolas,monospace;font-size:12px">${esc(i)}</td>\n      <td>${latHTML(t,e)}</td>\n      <td>${ago(t.last_seen)}</td>\n      <td>\n        <button class="btn" onclick="kickDevice('${esc(t.id)}','${esc(t.name)}')" ${n?"disabled":""}>下线</button>\n        ${n?`<button class="btn" onclick="enableDevice('${esc(t.id)}','${esc(t.name)}')">启用</button>`:`<button class="btn" onclick="disableDevice('${esc(t.id)}','${esc(t.name)}')">禁用</button>`}\n        <button class="btn" onclick="del('${esc(t.id)}','${esc(t.name)}')">移除</button>\n      </td>\n    </tr>`
    }).join("") : t.innerHTML = devices.length ? '<tr><td colspan="11" class="empty">没有匹配搜索条件的设备</td></tr>' : '<tr><td colspan="11" class="empty">暂无设备接入,请先在「账号」页创建账号</td></tr>'
}

function fillNetFilter() {
    const e = document.getElementById("dev-filter-net"),
        t = e.value;
    e.innerHTML = '<option value="">全部网络</option>' + networks.map(e => `<option value="${esc(e.id)}">${esc(e.name)}</option>`).join(""), [...e.options].some(e => e.value === t) && (e.value = t)
}

function renderNetworks() {
    const e = document.getElementById("netbody");
    networks.length ? e.innerHTML = networks.map(e => `\n    <tr>\n      <td>${esc(e.name)}</td>\n      <td class="mono">${esc(e.cidr)}</td>\n      <td>${e.device_count}</td>\n      <td>\n        <button class="btn" onclick="editNetwork('${esc(e.id)}','${esc(e.name)}','${esc(e.cidr)}',${"default"===e.id})">编辑</button>\n        ${"default"===e.id?'<span class="hint" style="margin:0">默认网络不可删除</span>':`<button class="del" onclick="delNetwork('${esc(e.id)}','${esc(e.name)}')">删除</button>`}\n      </td>\n    </tr>`).join("") : e.innerHTML = '<tr><td colspan="4" class="empty">暂无网络</td></tr>'
}
async function editNetwork(e, t, n, o) {
    openModal("编辑网络", {
        value: t,
        placeholder: "网络名称",
        onOK: async a => {
            if (closeModal(), a && a !== t) o ? await doEditNetwork(e, a, "") : openModal("编辑网络 CIDR", {
                value: n,
                placeholder: "CIDR,如 10.10.10.0/24",
                hint: "留空则保持不变",
                onOK: async t => {
                    closeModal(), await doEditNetwork(e, a, t && t !== n ? t : "")
                }
            });
            else {
                if (o) return;
                openModal("编辑网络 CIDR", {
                    value: n,
                    placeholder: "CIDR,如 10.10.10.0/24",
                    hint: "留空则保持不变",
                    onOK: async o => {
                        closeModal(), o && o !== n && await doEditNetwork(e, t, o)
                    }
                })
            }
        }
    })
}
async function doEditNetwork(e, t, n) {
    const o = {
        name: t
    };
    n && (o.cidr = n);
    await api("/api/networks/" + e, {
        method: "PUT",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify(o)
    }) && refresh()
}

function renderLinks() {
    const e = links.filter(e => "network" === e.type);
    document.getElementById("linkbody-net").innerHTML = e.length ? e.map(e => `\n    <tr>\n      <td>${esc(netName(e.a))}</td>\n      <td>↔</td>\n      <td>${esc(netName(e.b))}</td>\n      <td><button class="del" onclick="delLink('${esc(e.id)}')">删除</button></td>\n    </tr>`).join("") : '<tr><td colspan="4" class="empty">暂无网对网规则,不同网络互相隔离</td></tr>';
    const t = e => {
            const t = devices.find(t => t.id === e);
            return esc(t ? (t.name || t.virtual_ip) + " (" + t.virtual_ip + ")" : e)
        },
        n = links.filter(e => "device" === e.type);
    document.getElementById("linkbody-dev").innerHTML = n.length ? n.map(e => `\n    <tr>\n      <td>${t(e.a)}</td>\n      <td>↔</td>\n      <td>${t(e.b)}</td>\n      <td><button class="del" onclick="delLink('${esc(e.id)}')">删除</button></td>\n    </tr>`).join("") : '<tr><td colspan="4" class="empty">暂无点对点规则,设备保持隔离</td></tr>'
}

function genPassword() {
    const e = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789",
        t = new Uint32Array(10);
    crypto.getRandomValues(t), document.getElementById("acc-password").value = [...t].map(t => e[t % 62]).join("")
}

function showPassword(e) {
    e && (document.getElementById("acc-pw-text").textContent = e, document.getElementById("acc-pw-box").style.display = "block")
}
async function addAccount() {
    const e = document.getElementById("acc-username").value.trim(),
        t = document.getElementById("acc-password").value,
        n = document.getElementById("acc-network").value,
        o = parseInt(document.getElementById("acc-maxdev").value, 10) || 1;
    if (!e) return void alert("请填写用户名");
    if (!n) return void alert("请选择所属公司(网络)");
    const a = await fetch("/api/accounts", {
        method: "POST",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify({
            username: e,
            password: t,
            network_id: n,
            max_devices: o
        })
    });
    if (!a.ok) return void alert("创建失败 (" + a.status + "): " + (await a.text()).trim());
    showPassword((await a.json()).password), document.getElementById("acc-username").value = "", document.getElementById("acc-password").value = "", refresh()
}
async function resetPassword(e, t) {
    openModal("重置账号「" + t + "」的密码", {
        placeholder: "输入新密码(留空则随机生成)",
        random: !0,
        onOK: async t => {
            closeModal();
            const n = await fetch("/api/accounts/" + e + "/password", {
                method: "POST",
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify({
                    password: t
                })
            });
            n.ok ? (showPassword((await n.json()).password), refresh()) : alert("重置失败 (" + n.status + "): " + (await n.text()).trim())
        }
    })
}
async function setLimit(e, t, n) {
    openModal("调整账号「" + t + "」的设备数上限", {
        type: "number",
        value: String(n),
        placeholder: ">= 1 的整数",
        hint: "该账号下所有设备共享此上限",
        onOK: async t => {
            closeModal();
            const n = parseInt(t, 10);
            if (!n || n < 1) return void alert("上限必须是 >= 1 的整数");
            const o = await fetch("/api/accounts/" + e + "/limit", {
                method: "POST",
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify({
                    max_devices: n
                })
            });
            o.ok ? refresh() : alert("调整失败 (" + o.status + "): " + (await o.text()).trim())
        }
    })
}
async function delAccount(e, t) {
    confirm("确认删除账号「" + t + "」?") && (await api("/api/accounts/" + e, {
        method: "DELETE"
    }), refresh())
}

async function addRelay() {
    const e = document.getElementById("relay-address").value.trim(),
        t = document.getElementById("relay-pubkey").value.trim(),
        n = document.getElementById("relay-note").value.trim();
    if (!e) return void alert("请填写中转地址");
    (await api("/api/relays", {
        method: "POST",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify({
            address: e,
            pubkey: t,
            note: n
        })
    })) && (document.getElementById("relay-address").value = "", document.getElementById("relay-pubkey").value = "", document.getElementById("relay-note").value = "", refresh())
}
async function delRelay(e, t) {
    confirm("确认删除中转节点「" + t + "」?") && (await api("/api/relays/" + e, {
        method: "DELETE"
    }), refresh())
}
async function editRelay(e, t, n, o) {
    openModal("编辑中转节点", {
        value: t,
        placeholder: "公网地址:端口",
        hint: "可同时修改地址、公钥和备注",
        onOK: async a => {
            closeModal();
            const i = await fetch("/api/relays/" + e, {
                method: "PUT",
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify({
                    address: a || t,
                    pubkey: n,
                    note: o
                })
            });
            if (!i.ok) return void alert("修改失败 (" + i.status + "): " + (await i.text()).trim());
            refresh()
        }
    })
}

function renderRelays() {
    const e = document.getElementById("relaybody");
    relays.length ? e.innerHTML = relays.map(e => {
        const t = e.online ? '<span class="dot on"></span>在线' : '<span class="dot off"></span>离线';
        return `
    <tr>
      <td>${t}</td>
      <td class="mono">${esc(e.address)}</td>
      <td class="mono" title="${esc(e.pubkey||'')}">${esc((e.pubkey||'').slice(0,16)+(e.pubkey&&e.pubkey.length>16?'...':''))||"-"}</td>
      <td>${esc(e.note)||"-"}</td>
      <td>${ago(e.last_seen)}</td>
      <td>
        <button class="btn" onclick="editRelay('${esc(e.id)}','${esc(e.address)}','${esc(e.pubkey||'')}','${esc(e.note||'')}')">编辑</button>
        <button class="del" onclick="delRelay('${esc(e.id)}','${esc(e.address)}')">删除</button>
      </td>
    </tr>`
    }).join("") : e.innerHTML = '<tr><td colspan="6" class="empty">暂无中转节点，P2P 失败时客户端无法自动分配</td></tr>'
}

function renderAccounts() {
    const e = document.getElementById("acc-network"),
        t = e.value;
    e.innerHTML = networks.map(e => `<option value="${esc(e.id)}">${esc(e.name)} (${esc(e.cidr)})</option>`).join(""), [...e.options].some(e => e.value === t) && (e.value = t);
    const n = document.getElementById("accbody");
    accounts.length ? n.innerHTML = accounts.map(e => `\n    <tr>\n      <td>${esc(e.username)}</td>\n      <td>${esc(e.network_name)||"-"}</td>\n      <td>${e.device_count} / ${e.max_devices}</td>\n      <td>${ago(e.created_at)}</td>\n      <td><button class="btn" onclick="setLimit('${esc(e.id)}','${esc(e.username)}',${e.max_devices})">调整上限</button></td>\n      <td><button class="btn" onclick="resetPassword('${esc(e.id)}','${esc(e.username)}')">重置密码</button></td>\n      <td><button class="del" onclick="delAccount('${esc(e.id)}','${esc(e.username)}')">删除</button></td>\n    </tr>`).join("") : n.innerHTML = '<tr><td colspan="7" class="empty">暂无账号,客户端将无法注册</td></tr>'
}
async function refresh() {
    try {
        [devices, networks, links, accounts, relays] = await Promise.all([fetch("/api/devices").then(handleAuth), fetch("/api/networks").then(handleAuth), fetch("/api/links").then(handleAuth), fetch("/api/accounts").then(handleAuth), fetch("/api/relays").then(handleAuth)])
    } catch (e) {
        return void showLogin()
    }
    renderDevices(), fillNetFilter(), renderNetworks(), renderLinks(), fillLinkSelects(), renderAccounts(), renderRelays()
}

function startRefresh() {
    stopRefresh(), refresh(), refreshTimer = setInterval(refresh, 3e3)
}

function stopRefresh() {
    refreshTimer && (clearInterval(refreshTimer), refreshTimer = null)
}
async function handleAuth(e) {
    if (401 === e.status) throw showLogin(), new Error("未登录");
    return e.json()
}
const TABS = ["devices", "networks", "links-net", "links-dev", "accounts", "relays", "topology"];

function switchTab() {
    if (!isLoggedIn()) return;
    let e = location.hash.slice(1);
    if ("links" !== e) {
        TABS.includes(e) || (e = "devices");
        for (const t of TABS) document.getElementById("tab-" + t).classList.toggle("active", t === e), document.getElementById("page-" + t).classList.toggle("active", t === e);
        "topology" === e && loadTopology()
    } else location.replace("#links-net")
}

function isLoggedIn() {
    return document.getElementById("page-login").classList.contains("hidden")
}

function showLogin() {
    stopRefresh(), document.getElementById("page-login").classList.remove("hidden"), document.getElementById("logout-btn").style.display = "none", document.getElementById("change-password-btn").style.display = "none"
}

function showApp() {
    document.getElementById("page-login").classList.add("hidden"), document.getElementById("logout-btn").style.display = "inline-block", document.getElementById("change-password-btn").style.display = "inline-block"
}
async function checkSession() {
    try {
        if (!(await fetch("/api/admin/session")).ok) throw new Error("未登录");
        showApp(), switchTab(), startRefresh()
    } catch (e) {
        showLogin()
    }
}
async function login() {
    const e = document.getElementById("login-username").value.trim(),
        t = document.getElementById("login-password").value,
        n = document.getElementById("login-error");
    if (n.textContent = "", !e || !t) return void(n.textContent = "请输入用户名和密码");
    const o = md5Hex(t),
        a = await fetch("/api/admin/login", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({
                username: e,
                password: o
            })
        });
    a.ok ? (showApp(), switchTab(), startRefresh()) : n.textContent = (await a.text()).trim() || "登录失败"
}
async function logout() {
    await fetch("/api/admin/logout", {
        method: "POST"
    }), showLogin()
}
window.addEventListener("hashchange", switchTab), checkSession();
let topoChart = null,
    topoNodes = [],
    topoEdges = [],
    topoSelected = [],
    linkMode = !1,
    linkStart = null;
const TOPO_LAYOUT_PREFIX = "aleiyun_topology_layout_v1_";

function topologyLayoutKey(e) {
    return TOPO_LAYOUT_PREFIX + e
}

function initTopologyChart() {
    if (!window.echarts) return;
    const e = document.getElementById("topology-chart");
    e && (topoChart && topoChart.dispose(), topoChart = echarts.init(e, "dark", {
        renderer: "canvas",
        backgroundColor: "transparent"
    }), topoChart.on("click", onTopologyClick), topoChart.on("mousedown", e => {
        e && "node" === e.dataType && fixTopologyNode(e.data.id)
    }), topoChart.on("finished", () => {
        topoChart && (topoChart.resize(), saveTopologyLayout())
    }), topoChart.on("mouseup", () => {
        setTimeout(saveTopologyLayout, 50)
    }), window.addEventListener("resize", () => {
        topoChart && topoChart.resize()
    }))
}

function fixTopologyNode(e) {
    if (!topoChart) return;
    const t = topoChart.getOption(),
        n = t.series && t.series[0];
    if (!n || !n.data) return;
    let o = !1;
    n.data.forEach(t => {
        t.id === e && (t.fixed = !0, o = !0)
    }), o && topoChart.setOption({
        series: [{
            data: n.data
        }]
    })
}

function saveTopologyLayout() {
    if (!topoChart) return;
    const e = document.getElementById("topo-layout").value,
        t = topoChart.getOption(),
        n = t.series && t.series[0];
    if (!n || !n.data) return;
    const o = {};
    n.data.forEach(e => {
        null != e.x && null != e.y && (o[e.id] = {
            x: e.x,
            y: e.y
        })
    }), localStorage.setItem(topologyLayoutKey(e), JSON.stringify(o))
}

function loadTopologyLayout() {
    const e = document.getElementById("topo-layout").value;
    try {
        const t = localStorage.getItem(topologyLayoutKey(e));
        return t ? JSON.parse(t) : {}
    } catch (e) {
        return {}
    }
}

function resetTopologyLayout() {
    const e = document.getElementById("topo-layout").value;
    localStorage.removeItem(topologyLayoutKey(e)), renderTopology()
}
async function loadTopology() {
    window.echarts || await new Promise((e, t) => {
        const n = document.createElement("script");
        n.src = "https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js", n.onload = e, n.onerror = () => t(new Error("无法加载 ECharts")), document.head.appendChild(n)
    }), topoChart || initTopologyChart();
    try {
        const e = await fetch("/api/topology").then(e => e.json());
        topoNodes = e.nodes || [], topoEdges = e.edges || [], document.getElementById("topo-nodes").textContent = topoNodes.length, document.getElementById("topo-edges").textContent = topoEdges.length, renderTopology()
    } catch (e) {
        alert("加载拓扑图失败: " + e.message)
    }
}

function getVisibleEdges() {
    const e = document.getElementById("topo-show-belongs").checked,
        t = document.getElementById("topo-show-netlink").checked,
        n = document.getElementById("topo-show-devlink").checked,
        o = document.getElementById("topo-show-owns").checked;
    return topoEdges.filter(a => "belongs" === a.type ? e : "network-link" === a.type ? t : "device-link" === a.type ? n : "owns" !== a.type || o)
}

function renderTopology() {
    if (!topoChart) return;
    const e = document.getElementById("topo-layout").value,
        t = loadTopologyLayout(),
        n = [{
            name: "网络",
            itemStyle: {
                color: "#38bdf8"
            }
        }, {
            name: "设备",
            itemStyle: {
                color: "#34d399"
            }
        }, {
            name: "账号",
            itemStyle: {
                color: "#fbbf24"
            }
        }],
        o = new Set(topoSelected.map(e => e.id)),
        a = topoNodes.map(e => {
            const n = o.has(e.id),
                a = {
                    id: e.id,
                    name: e.name || e.id,
                    value: e.type,
                    category: "network" === e.type ? 0 : "device" === e.type ? 1 : 2,
                    symbolSize: "network" === e.type ? n ? 78 : 64 : "account" === e.type ? n ? 48 : 38 : n ? 36 : 28,
                    cursor: "pointer",
                    label: {
                        show: !0,
                        formatter: "device" === e.type ? e.name + "\n" + (e.virtual_ip || "") : e.name,
                        fontSize: 13,
                        color: "#ffffff",
                        fontWeight: n ? 700 : 500,
                        backgroundColor: n ? "rgba(15,23,42,0.7)" : "transparent",
                        padding: n ? [3, 6] : 0,
                        borderRadius: 4
                    },
                    data: e
                };
            if (t[e.id] && (a.x = t[e.id].x, a.y = t[e.id].y, a.fixed = !0), "device" === e.type) {
                let t = e.disabled ? "#ff5252" : e.online ? e.account_color || "#00e676" : "#b0bec5";
                a.itemStyle = {
                    color: t,
                    borderColor: "#ffffff",
                    borderWidth: n ? 4 : 2,
                    shadowBlur: n ? 28 : e.online ? 18 : 10,
                    shadowColor: n ? "#ffffff" : e.online ? t : "rgba(176,190,197,0.6)"
                }, e.disabled && (a.symbol = "rect")
            } else "network" === e.type ? a.itemStyle = {
                color: "#4fc3f7",
                borderColor: "#ffffff",
                borderWidth: n ? 4 : 2,
                shadowBlur: n ? 32 : 18,
                shadowColor: n ? "#ffffff" : "rgba(79,195,247,0.7)"
            } : "account" === e.type && (a.itemStyle = {
                color: e.account_color || "#ffd54f",
                borderColor: "#ffffff",
                borderWidth: n ? 4 : 2,
                shadowBlur: n ? 26 : 14,
                shadowColor: n ? "#ffffff" : "rgba(255,213,79,0.6)"
            });
            return a
        }),
        i = {
            belongs: "#00e5ff",
            owns: "#ffd54f",
            "network-link": "#e040fb",
            "device-link": "#ff4081"
        },
        s = getVisibleEdges().map(e => ({
            id: e.id,
            source: e.source,
            target: e.target,
            label: {
                show: "belongs" !== e.type && "owns" !== e.type,
                formatter: e.name || "",
                color: "#ffffff",
                fontSize: 12,
                fontWeight: 500
            },
            lineStyle: {
                type: "belongs" === e.type ? "solid" : "owns" === e.type ? "dotted" : "dashed",
                width: "belongs" === e.type ? 2 : 3,
                color: i[e.type] || "#b0bec5",
                opacity: "belongs" === e.type ? .65 : .95,
                curveness: .15
            },
            data: e
        })),
        c = {
            tooltip: {
                trigger: "item",
                backgroundColor: "rgba(15,23,42,0.95)",
                borderColor: "#475569",
                textStyle: {
                    color: "#ffffff"
                },
                formatter: e => {
                    if ("node" === e.dataType) {
                        const t = e.data.data;
                        return "network" === t.type ? `${t.name}<br/>CIDR: ${t.cidr}<br/>设备: ${t.device_count}` : "device" === t.type ? `${t.name}<br/>IP: ${t.virtual_ip}<br/>账号: ${t.username||"-"}<br/>状态: ${t.disabled?'<span style="color:#ff5252">已禁用</span>':t.online?'<span style="color:#00e676">在线</span>':'<span style="color:#b0bec5">离线</span>'}` : `${t.name}<br/>账号`
                    }
                    const t = e.data.data;
                    return `${t.name||t.type}: ${t.source} ↔ ${t.target}`
                }
            },
            legend: {
                data: n.map(e => e.name),
                textStyle: {
                    color: "#ffffff"
                },
                top: 8,
                itemGap: 20
            },
            animationDuration: 600,
            animationEasingUpdate: "quinticInOut",
            series: [{
                type: "graph",
                layout: "force" === e ? "force" : "none",
                data: a,
                links: s,
                categories: n,
                roam: !0,
                draggable: !0,
                label: {
                    position: "bottom",
                    color: "#ffffff",
                    distance: 10,
                    fontSize: 13
                },
                force: {
                    repulsion: 500,
                    gravity: .1,
                    edgeLength: [80, 220],
                    layoutAnimation: !0
                },
                emphasis: {
                    focus: "adjacency",
                    lineStyle: {
                        width: 5,
                        opacity: 1
                    },
                    itemStyle: {
                        shadowBlur: 28,
                        shadowColor: "inherit",
                        borderWidth: 3
                    }
                },
                lineStyle: {
                    curveness: .15
                },
                edgeSymbol: ["none", "arrow"],
                edgeSymbolSize: [0, 12]
            }]
        };
    if ("circular" === e) c.series[0].layout = "circular", c.series[0].circular = {
        rotateLabel: !1
    };
    else if ("grid" === e) {
        if (!(Object.keys(t).length > 0)) {
            const e = e => "network" === e ? 0 : "device" === e ? 1 : 2,
                t = a.slice().sort((t, n) => {
                    const o = e(t.data.type),
                        a = e(n.data.type);
                    return o !== a ? o - a : t.id < n.id ? -1 : 1
                }),
                n = Math.max(200, topoChart.getWidth()),
                o = Math.max(200, topoChart.getHeight()),
                i = 100,
                s = Math.max(200, n - 2 * i),
                c = Math.max(200, o - 2 * i),
                r = Math.max(...a.map(e => e.symbolSize || 40)) + 50,
                d = Math.ceil(Math.sqrt(t.length)),
                l = Math.ceil(t.length / d),
                p = (d - 1) * r,
                u = (l - 1) * r,
                m = Math.min(1, s / p, c / u),
                f = r * m,
                y = r * m,
                h = -(d - 1) * f / 2,
                g = -(l - 1) * y / 2;
            t.forEach((e, t) => {
                e.x = h + t % d * f, e.y = g + Math.floor(t / d) * y, e.fixed = !0
            })
        }
        c.series[0].layout = "none"
    }
    topoChart.setOption(c, !0), "grid" !== e && "circular" !== e || setTimeout(() => topoChart.dispatchAction({
        type: "restore"
    }), 50), topoSelected = [], document.getElementById("topology-info").style.display = "none"
}

function applyTopologyLayout() {
    renderTopology()
}

function onTopologyClick(e) {
    if ("node" === e.dataType) {
        const t = e.data.data;
        selectTopologyNode(t), showNodeInfo(t)
    } else "edge" === e.dataType ? showEdgeInfo(e.data.data) : (topoSelected = [], renderTopology(), document.getElementById("topology-info").style.display = "none")
}

function selectTopologyNode(e) {
    topoSelected = [e], renderTopology()
}

function isTopologySelected(e) {
    return topoSelected.some(t => t.id === e)
}

function showNodeInfo(e) {
    const t = document.getElementById("topology-info"),
        n = document.getElementById("topo-info-title"),
        o = document.getElementById("topo-info-body"),
        a = document.getElementById("topo-info-actions");
    t.style.display = "block", a.innerHTML = "", "network" === e.type ? (n.textContent = "网络: " + e.name, o.innerHTML = `CIDR: ${esc(e.cidr)}<br>设备数: ${e.device_count}`) : "device" === e.type ? (n.textContent = "设备: " + e.name, o.innerHTML = `虚拟 IP: ${esc(e.virtual_ip)}<br>账号: ${esc(e.username)||"-"}<br>状态: ${e.disabled?'<span style="color:#ef4444">已禁用</span>':e.online?'<span style="color:#22c55e">在线</span>':'<span style="color:#64748b">离线</span>'}`) : (n.textContent = "账号: " + e.name, o.innerHTML = `用户名: ${esc(e.name)}`), a.innerHTML = renderTopologyRecommendations(e)
}

function renderTopologyRecommendations(e) {
    if ("network" !== e.type && "device" !== e.type) return "";
    const t = "network" === e.type ? "network-link" : "device-link",
        n = "network" === e.type ? "network" : "device",
        o = e.id.replace("network" === e.type ? "net_" : "dev_", ""),
        a = new Set;
    for (const n of topoEdges) {
        if (n.type !== t) continue;
        const i = n.source.replace("network" === e.type ? "net_" : "dev_", ""),
            s = n.target.replace("network" === e.type ? "net_" : "dev_", "");
        i === o && a.add(s), s === o && a.add(i)
    }
    const i = topoNodes.filter(t => {
        if (t.type !== e.type || t.id === e.id) return !1;
        const n = t.id.replace("network" === e.type ? "net_" : "dev_", "");
        return !a.has(n)
    });
    if (!i.length) return `<div style="margin-top:10px;color:#64748b;font-size:12px">暂无可建立的${"network"===e.type?"网对网":"点对点"}连接</div>`;
    const s = "network" === e.type ? "可建立网对网" : "可建立点对点";
    return `<div style="margin-top:12px">\n    <div style="color:#38bdf8;font-size:13px;font-weight:600;margin-bottom:6px">${s}</div>\n    ${i.map(t=>`\n    <div style="display:flex;justify-content:space-between;align-items:center;margin:6px 0;padding:6px 8px;background:#0f172a;border:1px solid #334155;border-radius:4px">\n      <span style="color:#e2e8f0;font-size:12px">${(t=>"network"===e.type?`${esc(t.name)} (${esc(t.cidr)})`:`${esc(t.name)} (${esc(t.virtual_ip)})`)(t)}</span>\n      <button class="btn" style="padding:3px 10px;font-size:12px" onclick="addTopologyLinkByDrag('${n}','${o}','${(t=>t.id.replace("network"===e.type?"net_":"dev_",""))(t)}')">连接</button>\n    </div>\n  `).join("")}\n  </div>`
}

function showEdgeInfo(e) {
    const t = document.getElementById("topology-info"),
        n = document.getElementById("topo-info-title"),
        o = document.getElementById("topo-info-body"),
        a = document.getElementById("topo-info-actions");
    t.style.display = "block", n.textContent = "连线: " + (e.name || e.type);
    const i = topoNodes.find(t => t.id === e.source)?.name || e.source,
        s = topoNodes.find(t => t.id === e.target)?.name || e.target;
    if (o.innerHTML = `类型: ${esc(e.type)}<br>源: ${esc(i)}<br>目标: ${esc(s)}`, a.innerHTML = "", "network-link" === e.type || "device-link" === e.type) {
        const t = e.id.replace("link_", "");
        a.innerHTML = `<button class="del" onclick="delTopologyLink('${esc(t)}')">删除规则</button>`
    } else if ("belongs" === e.type) {
        const t = e.source.replace("dev_", "");
        a.innerHTML = `<button class="del" onclick="delTopologyDevice('${esc(t)}')">移除该设备</button>`
    } else if ("owns" === e.type) {
        const t = e.target.replace("dev_", "");
        a.innerHTML = `<button class="del" onclick="delTopologyDevice('${esc(t)}')">移除该设备</button>`
    }
}
async function delTopologyLink(e) {
    confirm("确认删除该互联规则?") && (await api("/api/links/" + e, {
        method: "DELETE"
    }), await refresh(), await loadTopology())
}
async function delTopologyDevice(e) {
    confirm("确认移除该设备?\n移除后该设备需重新注册，虚拟 IP 会被回收。") && (await api("/api/devices/" + e, {
        method: "DELETE"
    }), await refresh(), await loadTopology())
}
async function addTopologyLinkByDrag(e, t, n) {
    await api("/api/links", {
        method: "POST",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify({
            type: e,
            a: t,
            b: n
        })
    }) && (await refresh(), await loadTopology())
}

function searchTopology() {
    const e = document.getElementById("topo-search").value.trim().toLowerCase();
    if (!e || !topoChart) return;
    const t = topoNodes.find(t => (t.name || "").toLowerCase().includes(e) || (t.virtual_ip || "").includes(e));
    t && (topoChart.dispatchAction({
        type: "focusNodeAdjacency",
        seriesIndex: 0,
        name: t.name
    }), topoChart.dispatchAction({
        type: "showTip",
        seriesIndex: 0,
        name: t.name
    }))
}
let modalCallback = null;

function openModal(e, t = {}) {
    document.getElementById("modal-title").textContent = e;
    const n = document.getElementById("modal-input");
    n.type = t.type || "text", n.value = t.value || "", n.placeholder = t.placeholder || "", document.getElementById("modal-hint").textContent = t.hint || "";
    document.getElementById("modal-random").style.display = t.random ? "inline-block" : "none", modalCallback = t.onOK || null, document.getElementById("modal").classList.add("active"), setTimeout(() => n.focus(), 50)
}

function closeModal() {
    document.getElementById("modal").classList.remove("active"), modalCallback = null
}

function modalOK() {
    modalCallback && modalCallback(document.getElementById("modal-input").value.trim())
}

function fillModalRandom() {
    document.getElementById("modal-input").value = randomPassword()
}

function randomPassword() {
    const e = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
    let t = "";
    for (let n = 0; n < 10; n++) t += e.charAt(Math.floor(62 * Math.random()));
    return t
}
// 修改当前登录管理员密码
async function changePassword() {
  openModal("修改密码", {
    type: "password",
    placeholder: "旧密码",
    hint: "请输入旧密码",
    onOK: async (oldPassword) => {
      closeModal();
      if (!oldPassword) return;
      openModal("修改密码", {
        type: "password",
        placeholder: "新密码（不少于 6 位）",
        hint: "请输入新密码",
        onOK: async (newPassword) => {
          closeModal();
          if (!newPassword || newPassword.length < 6) {
            alert("新密码长度不能少于 6 位");
            return;
          }
          openModal("确认新密码", {
            type: "password",
            placeholder: "再次输入新密码",
            hint: "请再次输入新密码以确认",
            onOK: async (confirmPassword) => {
              closeModal();
              if (newPassword !== confirmPassword) {
                alert("两次输入的新密码不一致");
                return;
              }
              const resp = await fetch("/api/admin/password", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                  old_password: md5Hex(oldPassword),
                  new_password: md5Hex(newPassword)
                })
              });
              if (!resp.ok) {
                alert("修改密码失败: " + (await resp.text()).trim());
                return;
              }
              alert("密码修改成功，请重新登录");
              showLogin();
            }
          });
        }
      });
    }
  });
}

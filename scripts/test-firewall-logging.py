#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
"""Exercise real fw4/NFLOG in a disposable, isolated OpenWrt container.

Requires Docker's verso:dev image, Go, and build/verso-rpcd-amd64 (make build).
No host interfaces, sysctls, kernel log settings or existing containers change.
"""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parent.parent
NAME = f"verso-nflog-check-{os.getpid()}"


def command(*args, data=None, check=True):
    return subprocess.run(args, input=data, text=True, capture_output=True, check=check)


def inside(*args, data=None, check=True):
    return command("docker", "exec", "-i", NAME, *args, data=data, check=check)


def shell(script):
    return inside("sh", "-ec", script).stdout


def until(read, predicate, description):
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        value = read()
        if predicate(value):
            return value
        time.sleep(0.1)
    raise AssertionError(description)


def copy(source, destination, mode="0755"):
    command("docker", "cp", str(source), f"{NAME}:{destination}")
    shell(f"chown root:root {destination}; chmod {mode} {destination}")


def run():
    command("docker", "run", "-d", "--name", NAME, "--network", "none",
            "--cap-add", "NET_ADMIN", "--tmpfs", "/tmp", "verso:dev")
    until(lambda: inside("ubus", "call", "system", "board", check=False),
          lambda result: result.returncode == 0, "OpenWrt did not boot")
    # Wait past S94/S95 before replacing init scripts; otherwise the image's
    # remaining boot sequence could race setup and capture a patched baseline.
    until(lambda: inside("ubus", "call", "service", "list", '{"name":"verso"}', check=False),
          lambda r: r.returncode == 0 and any(i.get("running") for i in
                    json.loads(r.stdout).get("verso", {}).get("instances", {}).values()),
          "OpenWrt did not finish starting Verso")
    shell("/etc/init.d/verso-rpcd stop; mkdir -p /usr/libexec/verso")
    copy(ROOT / "build/verso-rpcd-amd64", "/usr/sbin/verso-rpcd")
    for path in ["etc/init.d/verso-rpcd", "usr/libexec/verso/firewall-logging",
                 "usr/libexec/verso/firewall-logging-setup"]:
        copy(ROOT / "docker/rootfs" / path, "/" + path)

    # A tiny test-only socket client runs inside the container. The shipped
    # helper protocol and real rpcd authorization remain under test.
    with tempfile.TemporaryDirectory(prefix="verso-nflog-client-") as temporary:
        client = Path(temporary) / "client.go"
        client.write_text('''package main
import ("encoding/json"; "net"; "os"; "time"; "io"; "fmt")
func main() {
 path:="/var/run/verso/verso-rpcd.sock"; if len(os.Args)>1 {path=os.Args[1]}
 c,e:=net.DialTimeout("unix",path,time.Second); if e!=nil {panic(e)}
 defer c.Close(); c.SetDeadline(time.Now().Add(5*time.Second))
 var r json.RawMessage; if e=json.NewDecoder(os.Stdin).Decode(&r); e!=nil {panic(e)}
 fmt.Fprintln(c,string(r)); io.Copy(os.Stdout,c)
}
''')
        env = dict(os.environ, CGO_ENABLED="0", GOOS="linux", GOARCH="amd64")
        subprocess.run(["go", "build", "-o", str(client.with_suffix("")), str(client)],
                       env=env, check=True, capture_output=True)
        copy(client.with_suffix(""), "/usr/bin/nflog-test-client")

    session = until(lambda: inside("ubus", "call", "session", "create", check=False),
                    lambda result: result.returncode == 0, "rpcd did not boot")
    original = json.loads(session.stdout)
    sid = original["ubus_rpc_session"]
    inside("ubus", "call", "session", "grant", json.dumps({
        "ubus_rpc_session": sid, "objects": [["verso", "firewallLog"]]}))

    def read(epoch="", after=-1, socket=None, session=None):
        args = ["/usr/bin/nflog-test-client"] + ([socket] if socket else [])
        return json.loads(inside(*args, data=json.dumps({"method": "firewallLog",
            "sid": sid if session is None else session,
            "args": {"generation": epoch, "after": str(after), "limit": "500"}})).stdout)

    templates = shell("sha256sum /usr/share/firewall4/templates/*.uc")
    shell("uci set firewall.pending_test=rule; uci set firewall.pending_test.enabled=0")
    pending = shell("uci changes firewall")
    shell("/usr/libexec/verso/firewall-logging-setup")
    assert pending == shell("uci changes firewall")
    assert "pending_test" not in shell("cat /etc/config/firewall")
    installed = shell("sha256sum /usr/share/firewall4/templates/*.uc")
    shell("/usr/libexec/verso/firewall-logging-setup")
    assert installed == shell("sha256sum /usr/share/firewall4/templates/*.uc")
    shell("/usr/libexec/verso/firewall-logging-setup remove")
    assert templates == shell("sha256sum /usr/share/firewall4/templates/*.uc")
    assert pending == shell("uci changes firewall")
    shell("uci revert firewall; /etc/init.d/verso-rpcd enable; /etc/init.d/verso-rpcd start")
    until(lambda: inside("test", "-S", "/var/run/verso/verso-rpcd.sock", check=False),
          lambda result: result.returncode == 0, "helper socket missing")
    first = until(read, lambda r: r.get("result", {}).get("available"), "NFLOG unavailable")
    assert read(session="0" * 32)["status"] == 6
    assert read(session="f" * 32)["status"] == 6

    shell('''uci set firewall.packet_test=rule
uci set firewall.packet_test.name=VERSO-NFLOG-MANAGED
uci set firewall.packet_test.dest='*'
uci set firewall.packet_test.proto=icmp
uci set firewall.packet_test.target=ACCEPT
uci set firewall.packet_test.log=1
uci set firewall.packet_test.log_limit=10/second
uci commit firewall
ip link add nflog-out type veth
ip link set nflog-out up
ip link set veth0 up
ip addr add 198.18.0.1/24 dev nflog-out
fw4 reload
fw4 check''')

    def assert_transport():
        generated = shell("fw4 print")
        logs = [line for line in generated.splitlines() if " log " in line]
        assert logs and all("group 4242" in line for line in logs), logs
        assert "limit rate 10/second" in generated
        rules = json.loads(shell("nft -j list table inet fw4"))["nftables"]
        statements = [e["log"] for item in rules for e in item.get("rule", {}).get("expr", []) if "log" in e]
        assert statements and all(log.get("group") == 4242 for log in statements), statements

    def probe():
        inside("ping", "-c", "1", "-W", "1", "198.18.0.2", check=False)

    assert_transport()
    probe()
    captured = until(read, lambda r: any("SRC=198.18.0.1 DST=198.18.0.2 PROTO=ICMP" in e["msg"]
                        for e in r["result"]["entries"]), "generated rule did not capture packet")
    assert "VERSO-NFLOG-MANAGED" not in shell("logread")
    # Different browsers may read the same retained history.
    repeated = read()["result"]
    assert captured["result"]["entries"][0] in repeated["entries"]
    epoch = first["result"]["generation"]
    shell("/etc/init.d/verso-rpcd stop")
    probe()
    assert_transport()
    assert "VERSO-NFLOG-MANAGED" not in shell("logread")
    shell("/etc/init.d/verso-rpcd start")
    until(lambda: inside("test", "-S", "/var/run/verso/verso-rpcd.sock", check=False),
          lambda result: result.returncode == 0, "helper did not restart")
    restarted = until(read, lambda r: r.get("result", {}).get("available"), "collector did not recover")
    assert restarted["result"]["generation"] != epoch
    probe()
    assert read(epoch, 999999)["result"]["reset"]
    shell("fw4 reload")
    assert_transport()
    probe()
    assert read()["result"]["entries"]

    # A second collector cannot steal the group and must report unavailable.
    shell("/usr/sbin/verso-rpcd --socket /var/run/verso/second.sock >/tmp/second.log 2>&1 & echo $! >/tmp/second.pid")
    until(lambda: inside("test", "-S", "/var/run/verso/second.sock", check=False),
          lambda result: result.returncode == 0, "second helper did not start")
    assert not read(socket="/var/run/verso/second.sock")["result"]["available"]
    shell("kill $(cat /tmp/second.pid)")
    assert_transport()

    # Broken routing must not disable unrelated privileged helper operations.
    shell('''cp /usr/share/firewall4/templates/rule.uc /usr/share/firewall4/templates/rule.backup
echo unsupported > /usr/share/firewall4/templates/rule.uc
/etc/init.d/verso-rpcd restart''')
    until(lambda: inside("test", "-S", "/var/run/verso/verso-rpcd.sock", check=False),
          lambda result: result.returncode == 0, "routing failure disabled the helper")
    assert not read()["result"]["available"]
    inside("ubus", "call", "session", "grant", json.dumps({
        "ubus_rpc_session": sid, "objects": [["verso", "firewallCounters"]]}))
    counters = json.loads(inside("/usr/bin/nflog-test-client", data=json.dumps({
        "method": "firewallCounters", "sid": sid})).stdout)
    assert counters["status"] == 0
    shell('''mv /usr/share/firewall4/templates/rule.backup /usr/share/firewall4/templates/rule.uc
/usr/libexec/verso/firewall-logging-setup''')
    until(read, lambda r: r["result"]["available"], "routing failure did not recover")

    # Boot ordering must preserve the generated transport with fresh RAM state.
    command("docker", "restart", NAME)
    session = until(lambda: inside("ubus", "call", "session", "create", check=False),
                    lambda result: result.returncode == 0, "rpcd did not return after reboot")
    sid = json.loads(session.stdout)["ubus_rpc_session"]
    inside("ubus", "call", "session", "grant", json.dumps({
        "ubus_rpc_session": sid, "objects": [["verso", "firewallLog"]]}))
    until(read, lambda r: r["result"]["available"], "collector did not start at boot")
    assert_transport()

    # Migration and removal preserve counters, rate limits and verdicts.
    shell('''nft add chain inet fw4 migration_test
nft add rule inet fw4 migration_test counter packets 17 bytes 900 limit rate 2/second log prefix LEGACY accept
/usr/libexec/verso/firewall-logging-setup''')
    live = shell("nft list chain inet fw4 migration_test")
    assert "counter packets 17 bytes 900" in live and "group 4242" in live and "accept" in live
    shell("/usr/libexec/verso/firewall-logging-setup remove")
    restored = shell("nft list chain inet fw4 migration_test")
    assert "counter packets 17 bytes 900" in restored and "group 4242" not in restored and "accept" in restored
    assert templates == shell("sha256sum /usr/share/firewall4/templates/*.uc")
    shell("nft delete table inet fw4; /usr/libexec/verso/firewall-logging-setup remove")
    print("PASS: NFLOG delivery, logd isolation, SID gate, reload/restart/boot, collector/routing failure, staged edits and reversible migration")


try:
    run()
except subprocess.CalledProcessError as failure:
    print(failure.stderr)
    raise
finally:
    command("docker", "rm", "-f", NAME, check=False)

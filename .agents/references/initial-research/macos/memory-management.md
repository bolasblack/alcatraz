# Memory Management: Auto-Release Unused Memory

**Status**: Initial research (降级为 Nice-to-have，非 MVP)

## Executive Summary

**结论**: 只有 **OrbStack** 真正支持自动释放未使用内存。Colima/Lima 虽然配置了 virtio-balloon，但实际上内存不会自动归还给 macOS。

| Solution | Auto-Release | Works? | Notes |
|----------|--------------|--------|-------|
| **OrbStack** | ✅ Yes | **Yes** | v1.7.0+ 有真正的动态内存管理 |
| **Colima VZ** | ❌ No | No | 配置了 balloon 但不工作 |
| **Lima VZ** | ❌ No | No | 同上，需要停止 VM 才能释放 |
| **Lima QEMU** | ❌ No | No | 需要手动干预 |
| **Docker Desktop** | ❌ No | No | 已知长期问题 |

---

## Colima/Lima Memory Behavior

### 现状

- Lima VZ 后端**确实配置了** `VZVirtioTraditionalMemoryBalloonDevice`
- Apple Virtualization.framework 支持 virtio-balloon 设备
- **但实际测试表明内存不会自动释放**

### 用户报告的问题

> "After running memory intensive tasks that maxed out at 8GB, the memory shows next to 'Virtual Machine Service for limactl' process in Activity Monitor stays at 8GB indefinitely until the VM is restarted."

来源: [lima-vm/lima Discussion #2720](https://github.com/lima-vm/lima/discussions/2720)

### 原因

1. Linux guest 将空闲内存用于 disk cache，不认为需要释放
2. 从 host 角度看，guest 占满了配额但不释放
3. Balloon 设备存在但没有主动回收机制

### Workarounds

```bash
# 在 guest 内清除缓存（临时方案）
echo 3 > /proc/sys/vm/drop_caches

# 最可靠的方式：停止 VM
colima stop
```

---

## OrbStack Memory Management

### 真正有效的动态内存

OrbStack v1.7.0 引入了专有的内存管理系统:

> "Memory is auto-released when no longer used, even while containers/machines are running."

来源: [OrbStack Blog - We fixed container memory usage on macOS](https://orbstack.dev/blog/dynamic-memory)

### 工作原理

1. 在 "giant array" 基础上追踪哪些 RAM 实际在用
2. 识别不再需要的内存部分
3. 在其他应用需要时自动释放

### 配置

```bash
# 设置最大内存限制（默认 8GB）
orb config set memory 8GiB

# 或在 GUI: Preferences > Resources
```

### 性能对比

| Metric | OrbStack | Docker Desktop |
|--------|----------|----------------|
| Idle RAM | ~1.1GB | 3-4GB |
| Memory release | Automatic | Manual/None |

---

## Docker Desktop Memory Issues

长期存在的问题，Virtualization.framework 启用后内存不会正确释放:

> "Docker process grows in memory usage and doesn't free it up properly."

来源: [docker/for-mac Issue #6120](https://github.com/docker/for-mac/issues/6120)

---

## 对 rcc MVP 的建议

### 如果需要内存自动释放

**推荐 OrbStack** - 唯一真正有效的方案

### 如果接受手动管理

**Colima VZ** 仍是好选择:
- 用户手动 `colima stop` 释放内存
- 或设置较低的内存限制避免过度占用

### MVP 配置示例 (Colima)

```bash
# 设置固定上限，用户接受手动管理
colima start \
  --cpu 4 \
  --memory 8 \    # 固定上限
  --vm-type vz \
  --mount-type virtiofs

# 需要释放内存时
colima stop
```

---

## 未完成的研究项目

以下项目因优先级变更未深入:

- [ ] 实际测试验证（stress-ng + vm_stat）
- [ ] 内存释放延迟基准测试
- [ ] Apple Containerization framework 内存行为
- [ ] Lima 2.0 external driver API 对内存管理的改进

---

## Sources

- [Lima Discussion #2720 - Memory not being freed](https://github.com/lima-vm/lima/discussions/2720)
- [OrbStack Blog - Dynamic Memory](https://orbstack.dev/blog/dynamic-memory)
- [OrbStack Docs - Efficiency](https://docs.orbstack.dev/efficiency)
- [Docker for Mac Issue #6120](https://github.com/docker/for-mac/issues/6120)
- [Apple VZVirtioTraditionalMemoryBalloonDevice](https://developer.apple.com/documentation/virtualization/vzvirtiotraditionalmemoryballoondevice)

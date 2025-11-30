const { createApp, ref, computed, onMounted, onUnmounted, watch } = Vue;

// API 基础路径
const API_BASE = '';  // 相对路径，自动使用当前域名

// API 认证配置
const API_KEY = 'sk_test_admin_key_for_testing_12345678'; // 生产环境 API Key

// 配置 axios 默认请求头
axios.defaults.headers.common['X-API-Key'] = API_KEY;  // 统一使用大写格式
axios.defaults.headers.common['Content-Type'] = 'application/json';

// --- Vue App ---
createApp({
    setup() {
        // 端口状态码定义（与后端 coremodel.PortStatusCode 保持一致）
        // 必须在最前面定义，因为后面的 computed 会使用
        // 0: offline  - 设备离线（不能充电）
        // 1: idle     - 空闲可用（唯一可以充电的状态）
        // 2: charging - 充电中（不能充电）
        // 3: fault    - 故障（不能充电）
        const PORT_STATUS = {
            OFFLINE: 0,
            IDLE: 1,
            CHARGING: 2,
            FAULT: 3
        };

        // State
        const apiConnected = ref(false);
        const devices = ref([]);
        const selectedDevice = ref(null);
        const selectedPort = ref(null);
        const logs = ref([]);
        const currentTime = ref('');
        const logContainer = ref(null);
        const autoScroll = ref(true);
        const loading = ref(false);
        const searchQuery = ref('');

        // 自动刷新配置
        const autoRefresh = ref(true);  // 自动刷新开关
        const refreshIntervalSeconds = ref(15);  // 刷新间隔（秒）
        const refreshIntervalOptions = [3, 5, 10, 15, 30, 60];  // 可选的刷新间隔

        // Charge Parameters
        const chargeParams = ref({
            socket_uid: '301015011402415',
            charge_mode: 1,  // 默认按时长
            amount: 100,     // 默认100分
            duration_minutes: 1,  // 默认60分钟
            port_no: 0,
            order_no: ''      // 当前端口订单号，停止充电必填
        });

        // 记录每个设备/端口最近使用的订单号，便于停止充电
        const portOrders = ref({});
        const currentOrderKey = computed(() => {
            if (!selectedDevice.value || selectedPort.value === null) {
                return null;
            }
            return `${selectedDevice.value.device_phy_id}:${selectedPort.value}`;
        });

        const syncOrderFields = () => {
            const key = currentOrderKey.value;
            const memo = key ? portOrders.value[key] : null;
            chargeParams.value.order_no = memo?.order_no || '';
        };

        const rememberOrder = (orderNo) => {
            const key = currentOrderKey.value;
            if (!key || !orderNo) return;
            portOrders.value[key] = {
                order_no: orderNo
            };
            chargeParams.value.order_no = orderNo;
        };

        watch([selectedDevice, selectedPort], () => {
            syncOrderFields();
        });

        // 手动修改订单号时，同步到当前端口的缓存
        watch(() => chargeParams.value.order_no, (newOrder) => {
            const key = currentOrderKey.value;
            if (!key) return;
            const trimmed = (newOrder || '').trim();
            if (!trimmed) {
                delete portOrders.value[key];
                return;
            }
            portOrders.value[key] = {
                order_no: trimmed
            };
        });

        let refreshInterval = null;
        let timeInterval = null;

        // Computed
        const filteredDevices = computed(() => {
            if (!searchQuery.value) return devices.value;
            return devices.value.filter(d =>
                d.device_phy_id.toLowerCase().includes(searchQuery.value.toLowerCase())
            );
        });

        // 当前选中端口的状态（用于控制按钮显示）
        const selectedPortStatus = computed(() => {
            if (!selectedDevice.value || selectedPort.value === null) {
                return null;
            }
            const port = selectedDevice.value.ports?.find(p => p.port_no === selectedPort.value);
            return port ? port.status : null;
        });

        // 判断是否可以启动充电
        // 核心规则：只有 status === 1 (idle/空闲) 才能充电
        const canStartCharge = computed(() => {
            return selectedPortStatus.value === PORT_STATUS.IDLE && selectedDevice.value?.is_online && chargeParams.value.socket_uid;
        });

        // 判断是否可以停止充电
        const canStopCharge = computed(() => {
            return selectedPortStatus.value === PORT_STATUS.CHARGING && selectedDevice.value?.is_online;
        });

        const getCurrentOrderInfo = () => {
            const key = currentOrderKey.value;
            const memo = key ? portOrders.value[key] : null;
            return {
                orderNo: (chargeParams.value.order_no || memo?.order_no || '').trim()
            };
        };

        // Methods
        const addLog = (type, message, payload = null) => {
            const time = new Date().toLocaleTimeString('zh-CN', { hour12: false });
            logs.value.push({ time, type, message, payload });

            if (logs.value.length > 100) {
                logs.value.shift();
            }

            if (autoScroll.value && logContainer.value) {
                setTimeout(() => {
                    logContainer.value.scrollTop = logContainer.value.scrollHeight;
                }, 50);
            }
        };

        const clearLogs = () => logs.value = [];
        const toggleAutoScroll = () => autoScroll.value = !autoScroll.value;

        const updateTime = () => {
            currentTime.value = new Date().toLocaleString('zh-CN');
        };

        const formatTime = (timeStr) => {
            if (!timeStr) return 'N/A';
            try {
                return new Date(timeStr).toLocaleString('zh-CN');
            } catch (e) {
                return timeStr;
            }
        };

        const getPortStatusText = (status) => {
            const statusMap = {
                [PORT_STATUS.OFFLINE]: '离线',
                [PORT_STATUS.IDLE]: '空闲',
                [PORT_STATUS.CHARGING]: '充电中',
                [PORT_STATUS.FAULT]: '故障'
            };

            return statusMap[status] || '未知';
        };

        const getPortStatusColor = (status) => {
            const colorMap = {
                [PORT_STATUS.OFFLINE]: 'gray',
                [PORT_STATUS.IDLE]: 'green',
                [PORT_STATUS.CHARGING]: 'yellow',
                [PORT_STATUS.FAULT]: 'red'
            };

            return colorMap[status] || 'gray';
        };

        const getDeviceStatusText = (device) => {
            if (!device.is_online) return '离线';

            // 检查是否有任何端口实际在充电（状态为 CHARGING）
            const hasChargingPort = device.ports && device.ports.some(port => port.status === PORT_STATUS.CHARGING);

            if (hasChargingPort) {
                return '充电中';
            }

            // 检查是否有故障端口
            const hasFaultPort = device.ports && device.ports.some(port => port.status === PORT_STATUS.FAULT);
            if (hasFaultPort) {
                return '故障';
            }

            return '空闲';
        };

        // API Calls
        const fetchDevices = async () => {
            try {
                const response = await axios.get(`${API_BASE}/api/v1/third/devices`);
                if (response.data.code === 0) {
                    apiConnected.value = true;
                    const devicesData = response.data.data || [];

                    // 计算每个设备的 activeOrdersCount
                    devices.value = devicesData.map(device => ({
                        ...device,
                        id: device.device_phy_id,  // 兼容模板中的 device.id
                        isOnline: device.is_online,  // 兼容模板中的 device.isOnline
                        activeOrdersCount: device.active_orders ? device.active_orders.length : 0
                    }));

                    // Update selected device if exists
                    if (selectedDevice.value) {
                        const updated = devices.value.find(d => d.device_phy_id === selectedDevice.value.device_phy_id);
                        if (updated) {
                            selectedDevice.value = updated;
                        }
                    }
                } else {
                    addLog('错误', `获取设备列表失败: ${response.data.message}`);
                }
            } catch (error) {
                apiConnected.value = false;
                console.error('Failed to fetch devices:', error);
                addLog('错误', `API 调用失败: ${error.message}`);
            }
        };

        const refreshData = async () => {
            loading.value = true;
            await fetchDevices();
            loading.value = false;
        };

        // 启动/停止自动刷新
        const setupRefreshInterval = () => {
            if (refreshInterval) {
                clearInterval(refreshInterval);
                refreshInterval = null;
            }

            if (autoRefresh.value) {
                refreshInterval = setInterval(refreshData, refreshIntervalSeconds.value * 1000);
                addLog('信息', `自动刷新已启用，间隔: ${refreshIntervalSeconds.value}秒`);
            } else {
                addLog('信息', '自动刷新已禁用');
            }
        };

        // 切换自动刷新
        const toggleAutoRefresh = () => {
            autoRefresh.value = !autoRefresh.value;
            setupRefreshInterval();
        };

        // 监听刷新间隔变化
        watch(refreshIntervalSeconds, () => {
            if (autoRefresh.value) {
                setupRefreshInterval();
            }
        });

        // Actions
        const selectDevice = (device) => {
            addLog('信息', `选择设备: ${device.device_phy_id}`);
            selectedDevice.value = device;
            selectedPort.value = device.ports && device.ports.length > 0 ? device.ports[0].port_no : null;
            syncOrderFields();
        };

        const startCharging = async () => {
            if (!selectedDevice.value || selectedPort.value === null) {
                addLog('警告', '请先选择设备和端口');
                return;
            }

            loading.value = true;

            // 生成或复用订单号：THD + 时间戳（毫秒）
            const { orderNo: memoOrderNo } = getCurrentOrderInfo();
            const orderNo = memoOrderNo || 'THD' + Date.now();
            chargeParams.value.order_no = orderNo;
            addLog('信息', `正在启动充电: ${selectedDevice.value.device_phy_id} 端口 ${selectedPort.value}，订单号: ${orderNo}`);

            try {
                const requestBody = {
                    socket_uid: chargeParams.value.socket_uid,
                    port_no: selectedPort.value,
                    charge_mode: chargeParams.value.charge_mode,
                    amount: chargeParams.value.amount,
                    order_no: orderNo
                };

                // 按时长模式需要传 duration_minutes
                if (chargeParams.value.charge_mode === 1) {
                    requestBody.duration_minutes = chargeParams.value.duration_minutes;
                }

                const response = await axios.post(
                    `${API_BASE}/api/v1/third/devices/${selectedDevice.value.device_phy_id}/charge`,
                    requestBody
                );

                if (response.data.code === 0) {
                    const responseData = response.data.data || {};
                    rememberOrder(responseData.order_no || orderNo);
                    addLog('成功', '充电启动成功', responseData);
                } else {
                    addLog('错误', `启动充电失败: ${response.data.message}`);
                }
            } catch (error) {
                console.error('Failed to start charging:', error);
                addLog('错误', `启动充电失败: ${error.response?.data?.message || error.message}`);
            } finally {
                // 点击启动后立即刷新设备状态，确保 third/devices 数据及时更新
                await refreshData();
                loading.value = false;
            }
        };

        const stopCharging = async () => {
            if (!selectedDevice.value || selectedPort.value === null) {
                addLog('警告', '请先选择设备和端口');
                return;
            }

            const { orderNo } = getCurrentOrderInfo();
            if (!orderNo) {
                addLog('警告', '停止充电失败: 缺少订单号，请先填写订单号或使用控制台启动充电以生成订单');
                return;
            }

            loading.value = true;
            addLog('信息', `正在停止充电: ${selectedDevice.value.device_phy_id} 端口 ${selectedPort.value}，订单号: ${orderNo}`);

            try {
                const payload = {
                    socket_uid: chargeParams.value.socket_uid,
                    port_no: selectedPort.value,
                    order_no: orderNo
                };

                const response = await axios.post(
                    `${API_BASE}/api/v1/third/devices/${selectedDevice.value.device_phy_id}/stop`,
                    payload  // 使用JSON body传递 socket_uid + port_no + order_no
                );

                if (response.data.code === 0) {
                    addLog('成功', '充电已停止', response.data.data || {});
                } else {
                    addLog('错误', `停止充电失败: ${response.data.message}`);
                }
            } catch (error) {
                console.error('Failed to stop charging:', error);
                addLog('错误', `停止充电失败: ${error.response?.data?.message || error.message}`);
            } finally {
                // 停止充电后立即刷新设备状态，确保 third/devices 数据更新
                await refreshData();
                loading.value = false;
            }
        };

        // Lifecycle
        onMounted(() => {
            addLog('信息', '系统初始化完成');
            updateTime();
            timeInterval = setInterval(updateTime, 1000);

            // 初始化获取设备列表
            fetchDevices();

            // 设置自动刷新
            setupRefreshInterval();
        });

        onUnmounted(() => {
            if (timeInterval) clearInterval(timeInterval);
            if (refreshInterval) clearInterval(refreshInterval);
        });

        return {
            apiConnected,
            devices,
            filteredDevices,
            selectedPortStatus,
            canStartCharge,
            canStopCharge,
            searchQuery,
            selectedDevice,
            selectedPort,
            logs,
            currentTime,
            logContainer,
            autoScroll,
            loading,
            chargeParams,
            autoRefresh,
            refreshIntervalSeconds,
            refreshIntervalOptions,
            PORT_STATUS,

            selectDevice,
            startCharging,
            stopCharging,
            refreshData,
            clearLogs,
            toggleAutoScroll,
            toggleAutoRefresh,
            formatTime,
            getPortStatusText,
            getPortStatusColor,
            getDeviceStatusText
        };
    }
}).mount('#app');

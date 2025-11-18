const { createApp } = Vue;

// API配置
const API_BASE_URL = '/internal/test';
const API_KEY = 'sk_test_admin_key_for_testing_12345678'; // 生产环境API Key

// Axios实例配置
axios.defaults.headers.common['X-Internal-API-Key'] = API_KEY;
axios.defaults.headers.common['Content-Type'] = 'application/json';

createApp({
    data() {
        return {
            // 视图状态
            currentView: 'devices', // devices, device-detail, timeline, sessions
            loading: false,
            timelineLoading: false,
            testRunning: false,

            // 设备数据
            devices: [],
            selectedDevice: null,
            searchQuery: '',
            filterStatus: '',

            // 测试场景
            scenarios: [],
            selectedScenario: null,

            // 测试参数
            testParams: {
                port_no: 1,
                charge_mode: 1,
                amount: 100,
                duration_minutes: 5,
                power: 0,
                price_per_kwh: 100,
                service_fee: 100
            },

            // 测试会话
            currentTestSessionId: null,
            currentTimelineSessionId: null,
            sessionSearchQuery: '',

            // 时间线数据
            timeline: null,
            expandedEvents: {},

            // 订单数据
            orders: [],
            selectedOrder: null,

            // 轮询控制
            pollingIntervals: {
                devices: null,
                deviceDetail: null,
                timeline: null,
                orders: null
            },
            pollingEnabled: true,
            lastUpdate: null,

            // 通知
            notification: null
        };
    },
    computed: {
        filteredDevices() {
            let filtered = this.devices;

            // 按搜索词过滤
            if (this.searchQuery) {
                const query = this.searchQuery.toLowerCase();
                filtered = filtered.filter(d =>
                    d.device_phy_id.toLowerCase().includes(query) ||
                    d.device_id.toString().includes(query)
                );
            }

            // 按状态过滤
            if (this.filterStatus) {
                filtered = filtered.filter(d => {
                    if (this.filterStatus === 'online') return d.is_online;
                    if (this.filterStatus === 'offline') return !d.is_online;
                    return true;
                });
            }

            return filtered;
        },

        // 获取有效端口列表（过滤掉port_no < 1的无效端口）
        validPorts() {
            if (!this.selectedDevice || !this.selectedDevice.ports) {
                return [];
            }
            return this.selectedDevice.ports.filter(p => p.port_no >= 1);
        },

        // 检查是否有有效端口
        hasValidPorts() {
            return this.validPorts.length > 0;
        }
    },
    methods: {
        // 加载设备列表
        async loadDevices() {
            this.loading = true;
            try {
                const response = await axios.get(`${API_BASE_URL}/devices`);
                if (response.data.code === 0) {
                    this.devices = response.data.data;
                    this.lastUpdate = new Date();
                    this.showNotification('设备列表加载成功', 'success');

                    // 启动设备列表轮询
                    if (this.currentView === 'devices' && this.pollingEnabled) {
                        this.startPollingDevices();
                    }
                } else {
                    throw new Error(response.data.message);
                }
            } catch (error) {
                console.error('加载设备失败:', error);
                this.showNotification('加载设备失败: ' + (error.response?.data?.message || error.message), 'error');
            } finally {
                this.loading = false;
            }
        },

        // 加载测试场景
        async loadScenarios() {
            try {
                const response = await axios.get(`${API_BASE_URL}/scenarios`);
                if (response.data.code === 0) {
                    this.scenarios = response.data.data;
                }
            } catch (error) {
                console.error('加载测试场景失败:', error);
            }
        },

        // 选择设备
        async selectDevice(device) {
            this.selectedDevice = device;
            this.currentView = 'device-detail';
            this.currentTestSessionId = null;

            // 停止设备列表轮询，启动设备详情轮询
            this.stopPollingDevices();
            if (this.pollingEnabled) {
                this.startPollingDeviceDetail();
            }

            // 自动选择第一个有效端口（port_no >= 1）
            const validPorts = (device.ports || []).filter(p => p.port_no >= 1);
            if (validPorts.length > 0) {
                this.testParams.port_no = validPorts[0].port_no;
            } else {
                // 没有有效端口时，设置为1作为默认值（会在UI上显示警告）
                this.testParams.port_no = 1;
            }

            // 如果还没有加载场景，加载它们
            if (this.scenarios.length === 0) {
                await this.loadScenarios();
            }
        },

        // 选择测试场景
        selectScenario(scenario) {
            this.selectedScenario = scenario;

            // 使用场景模板填充参数
            if (scenario.template) {
                Object.assign(this.testParams, scenario.template);
            }
        },

        // 开始测试
        async startTest() {
            if (!this.selectedDevice || !this.selectedScenario) {
                this.showNotification('请选择设备和测试场景', 'error');
                return;
            }

            this.testRunning = true;
            try {
                const payload = {
                    ...this.testParams,
                    scenario_id: this.selectedScenario.id
                };

                const response = await axios.post(
                    `${API_BASE_URL}/devices/${this.selectedDevice.device_phy_id}/charge`,
                    payload
                );

                if (response.data.code === 0) {
                    this.currentTestSessionId = response.data.data.test_session_id;
                    this.showNotification('测试启动成功!', 'success');

                    // 5秒后自动开始轮询时间线
                    setTimeout(() => {
                        this.startPollingTimeline();
                    }, 5000);
                } else {
                    throw new Error(response.data.message);
                }
            } catch (error) {
                console.error('启动测试失败:', error);
                this.showNotification('启动测试失败: ' + (error.response?.data?.message || error.message), 'error');
            } finally {
                this.testRunning = false;
            }
        },

        // 停止测试
        async stopTest() {
            if (!this.selectedDevice || !this.currentTestSessionId) {
                return;
            }

            try {
                const response = await axios.post(
                    `${API_BASE_URL}/devices/${this.selectedDevice.device_phy_id}/stop`,
                    { port_no: this.testParams.port_no }
                );

                if (response.data.code === 0) {
                    this.showNotification('停止充电指令已发送', 'success');
                } else {
                    throw new Error(response.data.message);
                }
            } catch (error) {
                console.error('停止测试失败:', error);
                this.showNotification('停止测试失败: ' + (error.response?.data?.message || error.message), 'error');
            }
        },

        // 查看时间线
        async viewTimeline(sessionId) {
            this.currentTimelineSessionId = sessionId;
            this.currentView = 'timeline';
            await this.loadTimeline(sessionId);
        },

        // 通过ID查看会话
        viewSessionById() {
            if (!this.sessionSearchQuery) {
                this.showNotification('请输入测试会话ID', 'error');
                return;
            }
            this.viewTimeline(this.sessionSearchQuery);
        },

        // 加载时间线
        async loadTimeline(sessionId) {
            this.timelineLoading = true;
            this.expandedEvents = {};
            try {
                const response = await axios.get(`${API_BASE_URL}/sessions/${sessionId}`);
                if (response.data.code === 0) {
                    this.timeline = response.data.data;
                } else {
                    throw new Error(response.data.message);
                }
            } catch (error) {
                console.error('加载时间线失败:', error);
                this.showNotification('加载时间线失败: ' + (error.response?.data?.message || error.message), 'error');
            } finally {
                this.timelineLoading = false;
            }
        },

        // 开始轮询时间线
        startPollingTimeline() {
            if (!this.currentTestSessionId) return;

            // 清除现有轮询
            if (this.pollingIntervals.timeline) {
                clearInterval(this.pollingIntervals.timeline);
            }

            this.pollingIntervals.timeline = setInterval(async () => {
                if (!this.pollingEnabled || this.currentView !== 'device-detail' || !this.currentTestSessionId) {
                    this.stopPollingTimeline();
                    return;
                }

                try {
                    await this.loadTimeline(this.currentTestSessionId);
                    this.lastUpdate = new Date();
                } catch (error) {
                    console.error('轮询时间线失败:', error);
                }
            }, 3000); // 每3秒轮询一次
        },

        // 停止轮询时间线
        stopPollingTimeline() {
            if (this.pollingIntervals.timeline) {
                clearInterval(this.pollingIntervals.timeline);
                this.pollingIntervals.timeline = null;
            }
        },

        // 开始轮询设备详情
        startPollingDeviceDetail() {
            if (!this.selectedDevice) return;

            // 清除现有轮询
            if (this.pollingIntervals.deviceDetail) {
                clearInterval(this.pollingIntervals.deviceDetail);
            }

            this.pollingIntervals.deviceDetail = setInterval(async () => {
                if (!this.pollingEnabled || this.currentView !== 'device-detail' || !this.selectedDevice) {
                    this.stopPollingDeviceDetail();
                    return;
                }

                try {
                    const response = await axios.get(`${API_BASE_URL}/devices/${this.selectedDevice.device_phy_id}`);
                    if (response.data.code === 0) {
                        this.selectedDevice = response.data.data;
                        this.lastUpdate = new Date();
                    }
                } catch (error) {
                    console.error('轮询设备详情失败:', error);
                }
            }, 5000); // 每5秒轮询一次
        },

        // 停止轮询设备详情
        stopPollingDeviceDetail() {
            if (this.pollingIntervals.deviceDetail) {
                clearInterval(this.pollingIntervals.deviceDetail);
                this.pollingIntervals.deviceDetail = null;
            }
        },

        // 开始轮询设备列表
        startPollingDevices() {
            // 清除现有轮询
            if (this.pollingIntervals.devices) {
                clearInterval(this.pollingIntervals.devices);
            }

            this.pollingIntervals.devices = setInterval(async () => {
                if (!this.pollingEnabled || this.currentView !== 'devices') {
                    this.stopPollingDevices();
                    return;
                }

                try {
                    const response = await axios.get(`${API_BASE_URL}/devices`);
                    if (response.data.code === 0) {
                        this.devices = response.data.data;
                        this.lastUpdate = new Date();
                    }
                } catch (error) {
                    console.error('轮询设备列表失败:', error);
                }
            }, 10000); // 每10秒轮询一次
        },

        // 停止轮询设备列表
        stopPollingDevices() {
            if (this.pollingIntervals.devices) {
                clearInterval(this.pollingIntervals.devices);
                this.pollingIntervals.devices = null;
            }
        },

        // 停止所有轮询
        stopAllPolling() {
            this.stopPollingTimeline();
            this.stopPollingDeviceDetail();
            this.stopPollingDevices();
        },

        // 切换轮询状态
        togglePolling() {
            this.pollingEnabled = !this.pollingEnabled;
            if (this.pollingEnabled) {
                this.showNotification('实时更新已启用', 'success');
                // 根据当前视图重启轮询
                if (this.currentView === 'devices') {
                    this.startPollingDevices();
                } else if (this.currentView === 'device-detail') {
                    this.startPollingDeviceDetail();
                    if (this.currentTestSessionId) {
                        this.startPollingTimeline();
                    }
                }
            } else {
                this.showNotification('实时更新已暂停', 'info');
                this.stopAllPolling();
            }
        },

        // 导出时间线
        exportTimeline() {
            if (!this.timeline) return;

            const dataStr = JSON.stringify(this.timeline, null, 2);
            const dataBlob = new Blob([dataStr], { type: 'application/json' });
            const url = URL.createObjectURL(dataBlob);
            const link = document.createElement('a');
            link.href = url;
            link.download = `timeline-${this.currentTimelineSessionId}.json`;
            link.click();
            URL.revokeObjectURL(url);

            this.showNotification('时间线数据已导出', 'success');
        },

        // 切换事件数据展开/收起
        toggleEventData(index) {
            this.expandedEvents[index] = !this.expandedEvents[index];
        },

        // 返回上一页
        goBack() {
            if (this.currentView === 'timeline') {
                if (this.selectedDevice) {
                    this.currentView = 'device-detail';
                    // 重启设备详情轮询
                    if (this.pollingEnabled) {
                        this.startPollingDeviceDetail();
                        if (this.currentTestSessionId) {
                            this.startPollingTimeline();
                        }
                    }
                } else {
                    this.currentView = 'sessions';
                }
            } else if (this.currentView === 'device-detail') {
                this.currentView = 'devices';
                this.selectedDevice = null;
                // 停止设备详情轮询，启动设备列表轮询
                this.stopPollingDeviceDetail();
                this.stopPollingTimeline();
                if (this.pollingEnabled) {
                    this.startPollingDevices();
                }
            }
        },

        // 显示通知
        showNotification(message, type = 'success') {
            this.notification = { message, type };
            setTimeout(() => {
                this.notification = null;
            }, 3000);
        },

        // 格式化日期
        formatDate(dateStr) {
            if (!dateStr) return '-';
            const date = new Date(dateStr);
            return date.toLocaleDateString('zh-CN');
        },

        // 格式化日期时间
        formatDateTime(dateStr) {
            if (!dateStr) return '-';
            const date = new Date(dateStr);
            return date.toLocaleString('zh-CN', {
                year: 'numeric',
                month: '2-digit',
                day: '2-digit',
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit'
            });
        },

        // 获取端口状态样式
        getPortStatusClass(status) {
            const classes = {
                0: 'bg-green-100 text-green-800',    // 空闲
                1: 'bg-yellow-100 text-yellow-800',  // 占用
                2: 'bg-red-100 text-red-800'         // 故障
            };
            return classes[status] || 'bg-gray-100 text-gray-800';
        },

        // 获取端口状态文本
        getPortStatusText(status) {
            const texts = {
                0: '空闲',
                1: '占用',
                2: '故障'
            };
            return texts[status] || '未知';
        },

        // 获取订单状态文本
        getOrderStatusText(status) {
            const texts = {
                0: 'pending',
                1: 'confirmed',
                2: 'charging',
                3: 'completed',
                4: 'failed',
                5: 'cancelled',
                6: 'refunded',
                7: 'settled',
                8: 'cancelling',
                9: 'stopping',
                10: 'interrupted'
            };
            return texts[status] || 'unknown';
        },

        // 获取事件边框样式
        getEventBorderClass(type) {
            const classes = {
                'http_request': 'border-blue-500',
                'db_operation': 'border-green-500',
                'outbound_cmd': 'border-purple-500',
                'device_report': 'border-orange-500',
                'event_push': 'border-pink-500',
                'error': 'border-red-500'
            };
            return classes[type] || 'border-gray-300';
        },

        // 获取事件图标
        getEventIcon(type) {
            const icons = {
                'http_request': 'fas fa-globe',
                'db_operation': 'fas fa-database',
                'outbound_cmd': 'fas fa-arrow-right',
                'device_report': 'fas fa-charging-station',
                'event_push': 'fas fa-paper-plane',
                'error': 'fas fa-exclamation-triangle'
            };
            return icons[type] || 'fas fa-circle';
        }
    },
    mounted() {
        // 页面加载时自动加载设备列表
        this.loadDevices();
        this.loadScenarios();

        // 检查URL中是否有session_id参数
        const urlParams = new URLSearchParams(window.location.search);
        const sessionId = urlParams.get('session_id');
        if (sessionId) {
            this.viewTimeline(sessionId);
        }
    },
    beforeUnmount() {
        // 清理所有轮询定时器
        this.stopAllPolling();
    }
}).mount('#app');

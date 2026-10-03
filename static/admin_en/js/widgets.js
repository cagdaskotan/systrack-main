// Dashboard Widget Fonksiyonları

// Response Time Trend Chart
function initResponseTimeTrendChart() {
    const ctx = document.getElementById('responseTimeTrendChart');
    if (!ctx) return;

    // Son 24 saatlik response time verilerini getir
    fetch('/api/pings/recent?limit=100')
        .then(response => response.json())
        .then(data => {
            // Veriyi saatlik gruplara ayır
            const hourlyData = {};
            const now = new Date();
            
            data.forEach(ping => {
                const pingTime = new Date(ping.timestamp);
                const hour = pingTime.getHours();
                
                if (!hourlyData[hour]) {
                    hourlyData[hour] = [];
                }
                hourlyData[hour].push(ping.response_time);
            });

            // Ortalama response time'ları hesapla
            const labels = [];
            const responseTimes = [];
            let totalResponseTime = 0;
            let maxResponseTime = 0;
            let minResponseTime = Infinity;
            let dataCount = 0;

            for (let hour = 0; hour < 24; hour++) {
                labels.push(`${hour}:00`);
                if (hourlyData[hour] && hourlyData[hour].length > 0) {
                    const avgTime = hourlyData[hour].reduce((a, b) => a + b, 0) / hourlyData[hour].length;
                    responseTimes.push(avgTime);
                    totalResponseTime += avgTime;
                    maxResponseTime = Math.max(maxResponseTime, avgTime);
                    minResponseTime = Math.min(minResponseTime, avgTime);
                    dataCount++;
                } else {
                    responseTimes.push(null);
                }
            }

            // İstatistikleri güncelle
            if (dataCount > 0) {
                const avgElement = document.getElementById('avgResponseTime');
                const maxElement = document.getElementById('maxResponseTime');
                const minElement = document.getElementById('minResponseTime');
                
                if (avgElement) avgElement.textContent = `${Math.round(totalResponseTime / dataCount)} ms`;
                if (maxElement) maxElement.textContent = `${Math.round(maxResponseTime)} ms`;
                if (minElement) minElement.textContent = `${Math.round(minResponseTime)} ms`;
            }

            // Chart.js ile grafik oluştur
            new Chart(ctx, {
                type: 'line',
                data: {
                    labels: labels,
                    datasets: [{
                        label: 'Response Time (ms)',
                        data: responseTimes,
                        borderColor: '#3b82f6',
                        backgroundColor: 'rgba(59, 130, 246, 0.1)',
                        borderWidth: 2,
                        fill: true,
                        tension: 0.4,
                        pointRadius: 3,
                        pointHoverRadius: 6
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        legend: {
                            display: false
                        }
                    },
                    scales: {
                        x: {
                            display: true,
                            title: {
                                display: true,
                                text: 'Saat'
                            }
                        },
                        y: {
                            display: true,
                            title: {
                                display: true,
                                text: 'Response Time (ms)'
                            },
                            beginAtZero: true
                        }
                    },
                    interaction: {
                        intersect: false,
                        mode: 'index'
                    }
                }
            });
        })
        .catch(error => {
            console.error('Response time trend chart error:', error);
        });
}

// En Yavaş Hedefler Widget'ı
function loadSlowestTargets() {
    const container = document.getElementById('slowestTargetsList');
    if (!container) return;

    fetch('/api/targets')
        .then(response => response.json())
        .then(targets => {
            // Her target için ortalama response time hesapla
            const targetPromises = targets.map(target => 
                fetch(`/api/pings/${target.id}?limit=50`)
                    .then(response => response.json())
                    .then(pings => {
                        if (pings.length === 0) return { ...target, avgResponseTime: 0 };
                        
                        const totalTime = pings.reduce((sum, ping) => sum + ping.response_time, 0);
                        const avgTime = totalTime / pings.length;
                        return { ...target, avgResponseTime: avgTime };
                    })
                    .catch(() => ({ ...target, avgResponseTime: 0 }))
            );

            Promise.all(targetPromises)
                .then(targetsWithTimes => {
                    // Response time'a göre sırala ve ilk 5'i al
                    const slowestTargets = targetsWithTimes
                        .sort((a, b) => b.avgResponseTime - a.avgResponseTime)
                        .slice(0, 5);

                    // HTML oluştur
                    container.innerHTML = slowestTargets.map((target, index) => `
                        <div class="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700 rounded-lg">
                            <div class="flex items-center space-x-3">
                                <div class="flex-shrink-0">
                                    <div class="w-8 h-8 bg-red-100 dark:bg-red-900 rounded-full flex items-center justify-center">
                                        <span class="text-sm font-semibold text-red-600 dark:text-red-400">${index + 1}</span>
                                    </div>
                                </div>
                                <div>
                                    <p class="text-sm font-medium text-gray-900 dark:text-gray-100">${target.name}</p>
                                    <p class="text-xs text-gray-500 dark:text-gray-400">${target.address}</p>
                                </div>
                            </div>
                            <div class="text-right">
                                <p class="text-sm font-semibold text-red-600 dark:text-red-400">${Math.round(target.avgResponseTime)} ms</p>
                                <p class="text-xs text-gray-500 dark:text-gray-400">ortalama</p>
                            </div>
                        </div>
                    `).join('');
                })
                .catch(error => {
                    console.error('Slowest targets error:', error);
                    container.innerHTML = '<p class="text-sm text-gray-500 dark:text-gray-400 text-center">Veri yüklenemedi</p>';
                });
        })
        .catch(error => {
            console.error('Targets fetch error:', error);
            container.innerHTML = '<p class="text-sm text-gray-500 dark:text-gray-400 text-center">Veri yüklenemedi</p>';
        });
}

// Kritik Hedefler Widget'ı
function loadCriticalTargets() {
    const container = document.getElementById('criticalTargetsList');
    if (!container) return;

    fetch('/api/analytics/sla/summary')
        .then(response => response.json())
        .then(data => {
            // SLA < 99% olan hedefleri bul
            const criticalTargets = data.targets ? data.targets.filter(target => 
                target.uptime_percentage < 99.0
            ).slice(0, 5) : [];

            if (criticalTargets.length === 0) {
                container.innerHTML = `
                    <div class="text-center py-8">
                        <div class="w-16 h-16 bg-green-100 dark:bg-green-900 rounded-full flex items-center justify-center mx-auto mb-4">
                            <svg class="w-8 h-8 text-green-600 dark:text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
                            </svg>
                        </div>
                        <p class="text-sm text-gray-500 dark:text-gray-400">Tüm hedefler SLA gereksinimlerini karşılıyor</p>
                    </div>
                `;
                return;
            }

            // HTML oluştur
            container.innerHTML = criticalTargets.map(target => `
                <div class="flex items-center justify-between p-3 bg-red-50 dark:bg-red-900/20 rounded-lg border border-red-200 dark:border-red-800">
                    <div class="flex items-center space-x-3">
                        <div class="flex-shrink-0">
                            <div class="w-8 h-8 bg-red-100 dark:bg-red-900 rounded-full flex items-center justify-center">
                                <svg class="w-4 h-4 text-red-600 dark:text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                                </svg>
                            </div>
                        </div>
                        <div>
                            <p class="text-sm font-medium text-gray-900 dark:text-gray-100">${target.name || 'Bilinmeyen Hedef'}</p>
                            <p class="text-xs text-gray-500 dark:text-gray-400">SLA İhlali</p>
                        </div>
                    </div>
                    <div class="text-right">
                        <p class="text-sm font-semibold text-red-600 dark:text-red-400">${target.uptime_percentage ? target.uptime_percentage.toFixed(2) : '0.00'}%</p>
                        <p class="text-xs text-gray-500 dark:text-gray-400">uptime</p>
                    </div>
                </div>
            `).join('');
        })
        .catch(error => {
            console.error('Critical targets error:', error);
            container.innerHTML = '<p class="text-sm text-gray-500 dark:text-gray-400 text-center">Veri yüklenemedi</p>';
        });
}

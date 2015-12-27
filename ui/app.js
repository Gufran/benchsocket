(function(exports, jq) {
    'use strict';

    exports.app = new Vue({
        el: '#app',

        data: {
            newServer: '',
            servers: []
        },

        methods: {
            addServer: function () {
                var addr = this.newServer.trim()
                if (addr == "") {
                    return
                }

                var srv = {
                    addr: addr,
                    state: 'initializing',
                    standby: 0,
                    active_connections: 0,
                    failed_connections: 0,
                    failed_requests: 0,
                    successful_requests: 0,
                    graceful_close: 0,
                    error_close: 0
                };

                this.newServer = '';

                var conn = new WebSocket("ws://"+addr+"/socket");
                conn.onclose = function(evt) {
                    srv.state = 'disconnected';
                }

                conn.onmessage = function(evt) {
                    var data = JSON.parse(evt.data)
                    for(var k in data) {
                        srv[k] = data[k];
                    }
                    srv.state = 'active';
                }
                
                srv.connector = conn
                this.servers.push(srv);

            }, // end addServer

            removeServer: function(index) {
                if(this.servers[index].connector) {
                    this.servers[index].connector.close()
                }
                this.servers.splice(index, 1);
            },

            bootServers: function() {
                for(var s in this.servers) {
                    jq.get("http://"+this.servers[s].addr+"/launch")
                }
            },

            startTraffic: function() {
                for(var s in this.servers) {
                    jq.get("http://"+this.servers[s].addr+"/begin")
                }
            },

            shutdownAll: function() {
                for(var s in this.servers) {
                    jq.get("http://"+this.servers[s].addr+"/end")
                }
            },
        }
    });
})(window, jQuery)

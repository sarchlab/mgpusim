(function(vis){

class Trace {

    constructor() {
        this.data = undefined;
        this.stageColor =  d3.scaleOrdinal()
            .domain([0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12])
            .range([
                'black', // unknown
                '#67001f', // fetch start
                'white', // fetch done
                '#b2182b',  // issue
                '#d6604d', // decode start
                'white', // decode done
                '#f4a582', // read start
                'white', // read done
                '#fddbc7', // exec start
                'white', // exec done
                '#92c5de', // write start
                'white', // write done
                '#4394c3', // wait mem
                '#2166ac',  // mem return
                '#053061', // complete
            ]);
        
        this.stageName = d3.scaleOrdinal()
            .domain([0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14])
            .range([
                'unknown',
                'fetch',
                'wait issue',
                'issue',
                'decode',
                'wait',
                'read',
                'wait',
                'exec',
                'wait',
                'write',
                'wait',
                'wait mem',
                'mem return',
                'complete'
            ]);
    }

    loadAndDraw(timeRange) {
        $.ajax({
            url: 'trace',
            method: 'GET',
            data: { start: timeRange[0], end: timeRange[1] },
            dataType: "json"
        }).done(function (data) {
            this.preprocess(data);
            this.render(data, timeRange[0], timeRange[1]);
        }.bind(this));
    }
    
    preprocess(data) {
        for (let i = 0; i < data.length; i++) {
            let inst = data[i];
            inst.startTime = inst.events[0].time;
            inst.endTime = inst.events[inst.events.length - 1].time;
            for (let j = 0; j < inst.events.length; j++) {
                let event = inst.events[j];
                event.instCount = i;
                event.inst = inst;
                if (j != inst.events.length - 1) {
                    let nextEvent = inst.events[j + 1];
                    event.endTime = nextEvent.time;
                } else {
                    event.endTime = event.time;
                }
            }
        }
    }
    
    render(data, startTime, endTime) {
        let tooltip = $('#tooltip');
        
        let width = window.innerWidth;
        let height = window.innerHeight - 200;

        let svg = d3.select('#pipeline-figure').selectAll('svg');
        let mainArea = svg.select('.main-area');

        let xScale = d3.scaleLinear()
            .domain([startTime, endTime])
            .range([0, width]);
        let widthScale = d3.scaleLinear()
            .domain([0, endTime - startTime])
            .range([0, width]);
        let instHeight = height / data.length;
        let xAxis = d3.axisBottom(xScale)
            .tickSize(height - 20)
            .tickFormat(function(d) {
                return d.toString();
            });
        svg.selectAll('.x-axis')
            // .attr("transform", "translate(0, 200)")
            .call(xAxis);


        let instBars = mainArea.selectAll('g')
            .data(data, function(d) {return d?d.id:this.id;});

        instBars.exit().remove();

        let instBarsEnter = instBars.enter()
            .append('g');

        let instStage = instBars.merge(instBarsEnter).selectAll('rect')
            .data(function (d) { return d.events; });

        instStage.exit().remove();

        let instStageEnter = instStage.enter()
            .append('rect')
            .attr('x', function (d) {
                return xScale(d.time);
            })
            .attr('y', function (d) { return d.instCount * instHeight; })
            .attr('width', function (d) {
                if (d.endTime < d.time) return 0;
                return widthScale(d.endTime - d.time);
            })
            .attr('height', instHeight * 0.7)
            .style('fill', function (d) {
                if (d.time == 0) {
                    return null;
                }
                return this.stageColor(d.stage);
            }.bind(this))
            .style('stroke', function (d) {
                if (d.time == 0) {
                    return null;
                }
                switch (d.stage) {
                    case 2: case 5: case 7: case 9: case 11:
                        return '#888888';
                }
                return null;
            });

        
        instStage.merge(instStageEnter)
            .transition()
            .attr('x', function (d) {
                return xScale(d.time);
            })
            .attr('y', function (d) { return d.instCount * instHeight; })
            .attr('width', function (d) {
                if (d.endTime < d.time) return 0;
                return widthScale(d.endTime - d.time);
            })
            .attr('height', instHeight * 0.7);
                        
        instStage.on("mouseover", function (d) {
                let content =
                    "wg: " + (d.inst.workgroup_id ? d.inst.workgroup_id : 0) +
                    ", wf: " + (d.inst.wavefront_id ? d.inst.wavefront_id : 0) +
                    ", simd: " + (d.inst.simd_id ? d.inst.simd_id : 0) +
                    "<br/>inst: " + d.inst.asm +
                    "<br/>stage: " + this.stageName(d.stage);
                tooltip.show()
                    .css({ left: d3.event.pageX, top: d3.event.pageY })
                    .html(content);
            }.bind(this))
            .on("mouseout", function (d) {
                tooltip.hide();
            })
            .on("click", function (d) {
                console.log(d);
            });
    }

};

vis.trace = new Trace();

})(window.vis = window.vis || {});

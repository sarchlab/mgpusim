(function () {
    'strict mode';

    function loadTraceData(start, end) {
        $.ajax({
            url: 'trace',
            method: 'GET',
            data: { start: 0, end: 100 },
            dataType: "json"
        }).done(function (data) {
            preprocess(data);
            console.log(data);
            visualize(data);
        });
    }

    function preprocess(data) {
        for (let i = 0; i < data.length; i++) {
            let inst = data[i];
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

    let scalingFactor = 1e10;
    let stageColor = d3.scaleOrdinal()
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
            '#4394c3', // complete
            '#2166ac', '#053061']);
    let stageName = d3.scaleOrdinal()
        .domain([0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12])
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
            'complete'
        ]);

    function visualize(data) {
        let tooltip = $('#tooltip');

        let svg = d3.select('#pipeline-figure').selectAll('svg');

            // .attr('width', window.innerWidth)
            // .attr('height', window.innerHeight);

        let mainArea = svg.append('g')
            .attr('viewBox', "50, 0, 1000, 500");

        mainArea.selectAll('g')
            .data(data)
            .enter()
            .append('g')
            .selectAll('rect')
            .data(function (d) { return d.events; })
            .enter()
            .append('rect')
            .attr('x', function (d) {
                return d.time * scalingFactor;
            })
            .attr('y', function (d) { return d.instCount * 10; })
            .attr('width', function (d) {
                if (d.endTime - d.time <= 1e-10) {
                    return 0;
                }
                return (d.endTime - d.time) * scalingFactor;
            })
            .attr('height', 7)
            .style('fill', function (d) {
                if (d.time == 0) {
                    return null;
                }
                return stageColor(d.stage);
            })
            .style('stroke', function (d) {
                if (d.time == 0) {
                    return null;
                }
                switch (d.stage) {
                    case 2: case 5: case 7: case 9: case 11:
                        return '#888888';
                }
                return null;
            })
            .on("mouseover", function (d) {
                let content =
                    "wg: " + (d.inst.workgroup_id ? d.inst.workgroup_id : 0) +
                    ", wf: " + (d.inst.wavefront_id ? d.inst.wavefront_id : 0) +
                    ", simd: " + (d.inst.simd_id ? d.inst.simd_id : 0) +
                    "<br/>inst: " + d.inst.asm +
                    "<br/>stage: " + stageName(d.stage);
                tooltip.show()
                    .css({ left: d3.event.pageX, top: d3.event.pageY })
                    .html(content);
            })
            .on("mouseout", function (d) {
                tooltip.hide();
            })
            .on("click", function (d) {
                console.log(d);
            });

    }

    $(document).ready(function () {
        resize();
        $(window).resize(function () {
            resize();
        });
        loadTraceData(0, 100);
    });

    function loadMinimapData() {

    }

    function resize() {
        let windowHeight = $(window).height();
        let windowWidth = $(window).width();

        $('#full-screen')
            .height(windowHeight)
            .width(windowWidth);

        $('#minimap')
            .height(200)
            .width(windowWidth);

        $('#pipeline-figure')
            .height(windowHeight - 200)
            .width(windowWidth);
    }
})();
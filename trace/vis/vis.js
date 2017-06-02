(function() {
    'strict mode';

    function loadTraceData(start, end) {
        $.ajax({
            url:'trace', 
            method: 'GET',
            data: {start:0, end:100},
            dataType: "json"
        }).done(function(data){
            preprocess(data)
            console.log(data);
            visualize(data);
        });
    }

    function preprocess(data) {
        for(let i = 0; i < data.length; i++) {
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
        .range(['black', 
            '#E1F5FE',
            '#B3E5FC',
            '#81D4FA',
            '#4FC3F7',
            '#29B6F6',
            '#03A9F4',
            '#039BE5',
            '#0288D1',
            '#0277BD',
            '#01579B'
        ])
        .domain([0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);
    function visualize(data) {
        let tooltip = $('#tooltip')

        let svg = d3.select('#figure').append('svg')
            .attr('width', window.innerWidth)
            .attr('height', window.innerHeight);

        svg.selectAll('.bar')
            .data(data)
            .enter()
            .append('g')
                .selectAll('rect')
                .data(function(d) {return d.events;}) 
                .enter()
                .append('rect')
                    .attr('x', function(d){
                        return d.time * scalingFactor;
                    })
                    .attr('y', function(d){return d.instCount * 10;})
                    .attr('width', function(d){
                        return (d.endTime - d.time) * scalingFactor;
                    })
                    .attr('height', 9)
                    .style('fill', function(d) {
                        return stageColor(d.stage);
                    })
                    .on("mouseover", function(d) {
                        tooltip.show()
                            .css({left:d3.event.pageX, top:d3.event.pageY})
                            .html("inst: " + d.inst.asm + 
                                ", \nstage: " + d.stage);
                    })
                    .on("mouseout", function(d) {
                        tooltip.hide();
                    })

    }

    $(document).ready(function() {
        loadTraceData(0, 100);
    });
    

})();
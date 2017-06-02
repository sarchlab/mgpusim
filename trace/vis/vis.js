(function() {
    'strict mode';

    function loadTraceData(start, end) {
        $.ajax({
            url:'trace', 
            method: 'GET',
            data: {start:0, end:100},
            dataType: "json"
        }).done(function(data){
            console.log(data);
            visualize(data);
        });
    }

    let scalingFactor = 1e10;
    function visualize(data) {
        var svg = d3.select('#figure').append('svg');
        svg.selectAll('.bar')
            .data(data)
            .enter()
            .append('rect')
                .attr('x', function(d){
                    return d.events[0].time * scalingFactor;
                })
                .attr('y', function(d, i){return i * 10;})
                .attr('width', function(d){
                    return (d.events[1].time - d.events[0].time) * scalingFactor;
                })
                .attr('height', 9);
    }

    $(document).ready(function() {
        loadTraceData(0, 100);
    });
    

})();
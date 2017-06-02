(function() {
    'strict mode';

    function loadTraceData(start, end) {
        $.ajax({
            url:'trace', 
            method: 'GET',
            data: {start:0, end:100}
        }).done(function(data){
            console.log(data);
        });
    }

    $(document).ready(function() {
        loadTraceData(0, 100);
    });
    

})();
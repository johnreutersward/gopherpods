$(document).ready(function() {
	$('#podcasts').dataTable({
		"paging":   false,
		"info":     false,
		"filter":   false,
		"order":    [[ 2, "desc" ]]
	});
});